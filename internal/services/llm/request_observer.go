package llm

import (
	"context"
	"errors"
)

// RequestObserver surrounds every provider request, including compaction and
// approval review. The finish function must release its reservation on failures.
type RequestObserver func(context.Context, ModelRuntimeConfig, []ChatMessage, []ToolDef) (func(*StreamResult, error) error, error)
type observerKey struct{}

func WithRequestObserver(ctx context.Context, observer RequestObserver) context.Context {
	return context.WithValue(ctx, observerKey{}, observer)
}

func AppendRequestObserver(ctx context.Context, observers ...RequestObserver) context.Context {
	previous, _ := ctx.Value(observerKey{}).(RequestObserver)
	if previous != nil {
		observers = append([]RequestObserver{previous}, observers...)
	}
	return context.WithValue(ctx, observerKey{}, ChainRequestObservers(observers...))
}

func ChainRequestObservers(observers ...RequestObserver) RequestObserver {
	return func(ctx context.Context, config ModelRuntimeConfig, messages []ChatMessage, tools []ToolDef) (func(*StreamResult, error) error, error) {
		finishers := make([]func(*StreamResult, error) error, 0, len(observers))
		for _, observer := range observers {
			if observer == nil {
				continue
			}
			finish, err := observer(ctx, config, messages, tools)
			if err != nil {
				for i := len(finishers) - 1; i >= 0; i-- {
					_ = finishers[i](nil, err)
				}
				return nil, err
			}
			if finish != nil {
				finishers = append(finishers, finish)
			}
		}
		return func(result *StreamResult, requestErr error) error {
			var finishErr error
			for i := len(finishers) - 1; i >= 0; i-- {
				finishErr = errors.Join(finishErr, finishers[i](result, requestErr))
			}
			return finishErr
		}, nil
	}
}

type requestConfigsKey struct{}
type requestConfig struct{ value ModelRuntimeConfig }

func ReplaceRequestConfig(ctx context.Context, config ModelRuntimeConfig) {
	if current, _ := ctx.Value(requestConfigsKey{}).(*requestConfig); current != nil {
		current.value = config
	}
}

type observedProvider struct {
	Provider
	factory *Factory
}

func (p observedProvider) StreamChatWithTools(ctx context.Context, c ModelRuntimeConfig, m []ChatMessage, t []ToolDef, cb StreamCallbacks) (result *StreamResult, err error) {
	local, _ := ctx.Value(observerKey{}).(RequestObserver)
	var observer RequestObserver
	if p.factory != nil {
		observer = p.factory.observer
	}
	if local != nil {
		observer = ChainRequestObservers(observer, local)
	}
	if observer == nil {
		return p.Provider.StreamChatWithTools(ctx, c, m, t, cb)
	}
	resolved := &requestConfig{value: c}
	observerCtx := context.WithValue(ctx, requestConfigsKey{}, resolved)
	finish, e := observer(observerCtx, c, m, t)
	if e != nil {
		return nil, e
	}
	defer func() {
		if finish != nil {
			if e := finish(result, err); err == nil {
				err = e
			}
		}
	}()
	return p.Provider.StreamChatWithTools(ctx, resolved.value, m, t, cb)
}

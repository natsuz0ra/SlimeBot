package llm

import "context"

// RequestObserver surrounds every provider request, including compaction and
// approval review. The finish function must release its reservation on failures.
type RequestObserver func(context.Context, ModelRuntimeConfig, []ChatMessage, []ToolDef) (func(*StreamResult, error) error, error)
type observerKey struct{}

func WithRequestObserver(ctx context.Context, observer RequestObserver) context.Context {
	return context.WithValue(ctx, observerKey{}, observer)
}

type observedProvider struct{ Provider }

func (p observedProvider) StreamChatWithTools(ctx context.Context, c ModelRuntimeConfig, m []ChatMessage, t []ToolDef, cb StreamCallbacks) (result *StreamResult, err error) {
	observer, _ := ctx.Value(observerKey{}).(RequestObserver)
	if observer == nil {
		return p.Provider.StreamChatWithTools(ctx, c, m, t, cb)
	}
	finish, e := observer(ctx, c, m, t)
	if e != nil {
		return nil, e
	}
	defer func() {
		if e := finish(result, err); err == nil {
			err = e
		}
	}()
	return p.Provider.StreamChatWithTools(ctx, c, m, t, cb)
}

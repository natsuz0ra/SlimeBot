package privacy

import (
	"context"
	"errors"
	"time"

	"slimebot/internal/services/llm"
)

// ProtectText runs before title/review truncation, so cutting a PEM block or a
// token cannot expose a fragment which no complete-value rule can recognize.
func (s *Service) ProtectText(ctx context.Context, cfg llm.ModelRuntimeConfig, text string) (out string, err error) {
	c, err := s.GetConfig(ctx)
	if err != nil {
		return "", err
	}
	if !c.Enabled {
		return text, nil
	}
	for _, id := range c.BypassProviderIDs {
		if id == cfg.ProviderID {
			return text, nil
		}
	}
	if llm.PrivacyScopeFromContext(ctx).SessionID == "" {
		return "", errors.New("privacy scope is required before truncating model input")
	}
	t, err := s.begin(ctx, c, false)
	if err != nil {
		return "", err
	}
	t.suppressRecord = true
	defer func() {
		if recover() != nil {
			out = ""
			err = errors.New("privacy input preparation failed")
		}
		t.finish(err != nil)
	}()
	if c.ModelEnabled {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.ModelTimeoutSeconds)*time.Second)
		defer cancel()
	}
	if _, err = t.maskText(ctx, text, ""); err != nil {
		return "", err
	}
	if t.record.Blocked {
		return "", ErrBlocked
	}
	t.processed = 0
	out, err = t.maskText(ctx, text, "")
	if err == nil && t.record.Blocked {
		err = ErrBlocked
	}
	return out, err
}

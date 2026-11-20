package llm

import "context"

type privacyScopeKey struct{}

type PrivacySummary struct {
	Masked  int  `json:"masked"`
	Warned  int  `json:"warned"`
	Blocked bool `json:"blocked"`
}

// PrivacyScope identifies local state. Unscoped auxiliary calls get a separate
// request scope; callers must never share a restore map between sessions.
type PrivacyScope struct {
	SessionID string
	RequestID string
	Source    string
}

func WithPrivacyScope(ctx context.Context, sessionID, requestID, source string) context.Context {
	return context.WithValue(ctx, privacyScopeKey{}, PrivacyScope{sessionID, requestID, source})
}

func PrivacyScopeFromContext(ctx context.Context) PrivacyScope {
	scope, _ := ctx.Value(privacyScopeKey{}).(PrivacyScope)
	return scope
}

package anthropic

import (
	"net/http"
	"testing"
)

func TestNewAnthropicClientUsesPhaseTimeouts(t *testing.T) {
	client := NewAnthropicClient()
	if client.Client.Timeout != 0 {
		t.Fatalf("whole-request timeout = %v, want 0", client.Client.Timeout)
	}
	transport, ok := client.Client.Transport.(*http.Transport)
	if !ok || transport.ResponseHeaderTimeout <= 0 || transport.TLSHandshakeTimeout <= 0 || transport.IdleConnTimeout <= 0 || transport.DialContext == nil {
		t.Fatalf("phase-scoped transport not configured: %+v", client.Client.Transport)
	}
}

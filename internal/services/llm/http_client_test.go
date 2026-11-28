package llm

import (
	"net/http"
	"testing"
	"time"
)

func TestNewStreamingHTTPClientUsesPhaseTimeouts(t *testing.T) {
	client := NewStreamingHTTPClient()
	if client.Timeout != 0 {
		t.Fatalf("whole-request timeout = %v, want 0", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.DialContext == nil || transport.TLSHandshakeTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 || transport.IdleConnTimeout <= 0 {
		t.Fatalf("phase timeouts are incomplete: %+v", transport)
	}
	if transport.ResponseHeaderTimeout > 5*time.Minute {
		t.Fatalf("unexpected response header timeout: %v", transport.ResponseHeaderTimeout)
	}
}

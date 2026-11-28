package llm

import (
	"net"
	"net/http"
	"time"
)

// NewStreamingHTTPClient applies timeouts to connection establishment phases,
// not to the whole response. The turn context owns the overall stream lifetime,
// so long thinking and long streaming responses are not truncated mid-body.
func NewStreamingHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 2 * time.Minute,
			ExpectContinueTimeout: time.Second,
		},
	}
}

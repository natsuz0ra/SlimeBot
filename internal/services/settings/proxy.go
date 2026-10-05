package settings

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slimebot/internal/apperrors"
	"slimebot/internal/runtime"
	"strings"
	"syscall"
	"time"
)

const proxyTestTarget = "https://api.github.com"

type ProxyTestResult struct {
	Success    bool   `json:"success"`
	Route      string `json:"route"`
	TargetURL  string `json:"targetUrl"`
	LatencyMs  int64  `json:"latencyMs"`
	StatusCode int    `json:"statusCode,omitempty"`
	ErrorCode  string `json:"errorCode,omitempty"`
}

func (s *SettingsService) TestProxy(ctx context.Context, raw string) (*ProxyTestResult, error) {
	return testProxy(ctx, raw, proxyTestTarget)
}

func testProxy(ctx context.Context, raw, target string) (*ProxyTestResult, error) {
	transport, err := runtime.NewProxyTransport(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
	}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SlimeBot")
	result := &ProxyTestResult{Route: "direct", TargetURL: target}
	proxy, err := transport.Proxy(req)
	if err != nil {
		return nil, err
	}
	if proxy != nil {
		result.Route = "proxy"
		if strings.TrimSpace(raw) == "" {
			result.Route = "environment"
		}
	}
	// Report the route used by this request, even if settings change during the test.
	transport.Proxy = http.ProxyURL(proxy)
	transport.OnProxyConnectResponse = func(_ context.Context, _ *url.URL, _ *http.Request, response *http.Response) error {
		if response.StatusCode != http.StatusOK {
			result.StatusCode = response.StatusCode
		}
		return nil
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	start := time.Now()
	response, err := client.Do(req)
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		result.ErrorCode = proxyTestErrorCode(err)
	} else {
		response.Body.Close()
		result.StatusCode = response.StatusCode
		result.Success = response.StatusCode >= 200 && response.StatusCode < 400
		if !result.Success {
			result.ErrorCode = "http"
		}
	}
	if result.StatusCode == http.StatusProxyAuthRequired {
		result.ErrorCode = "proxy_auth"
	} else if err != nil && result.StatusCode != 0 {
		result.ErrorCode = "proxy_http"
	}
	return result, nil
}

// Return stable categories instead of raw errors that can include environment credentials.
func proxyTestErrorCode(err error) string {
	var networkError net.Error
	var dnsError *net.DNSError
	var unknownAuthority x509.UnknownAuthorityError
	var invalidCertificate x509.CertificateInvalidError
	var hostnameError x509.HostnameError
	var tlsHeaderError tls.RecordHeaderError
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout():
		return "timeout"
	case errors.As(err, &dnsError):
		return "dns"
	case errors.As(err, &unknownAuthority) || errors.As(err, &invalidCertificate) || errors.As(err, &hostnameError) || errors.As(err, &tlsHeaderError):
		return "tls"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	default:
		return "network"
	}
}

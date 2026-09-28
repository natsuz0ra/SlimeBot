package runtime

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const ProxyEnvKey = "SLIMEBOT_PROXY_URL"

var proxyURL atomic.Pointer[url.URL]
var proxyTransportOnce sync.Once

// SetProxyURL applies the application's outbound proxy to default Go HTTP clients.
// An empty value restores the process environment's proxy behavior.
func SetProxyURL(raw string) error {
	parsed, err := parseProxyURL(raw)
	if err != nil {
		return err
	}
	proxyTransportOnce.Do(func() {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = func(req *http.Request) (*url.URL, error) {
			host := req.URL.Hostname()
			ip := net.ParseIP(host)
			if strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback() {
				return nil, nil
			}
			if configured := proxyURL.Load(); configured != nil {
				return configured, nil
			}
			return http.ProxyFromEnvironment(req)
		}
		http.DefaultTransport = transport
	})
	proxyURL.Store(parsed)
	return nil
}

func ValidateProxyURL(raw string) error {
	_, err := parseProxyURL(raw)
	return err
}

func parseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	var parsed *url.URL
	if raw != "" {
		var err error
		parsed, err = url.Parse(raw)
		if err != nil || parsed == nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("invalid proxy URL")
		}
		switch parsed.Scheme {
		case "http", "https", "socks5":
		default:
			return nil, fmt.Errorf("proxy URL must use http, https, or socks5")
		}
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("proxy URL requires a valid port")
		}
	}
	return parsed, nil
}

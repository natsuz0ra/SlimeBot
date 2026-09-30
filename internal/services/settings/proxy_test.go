package settings

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slimebot/internal/apperrors"
	"slimebot/internal/runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyUsesDraftWithoutChangingLiveProxy(t *testing.T) {
	liveProxy := "http://127.0.0.1:12345"
	if err := runtime.SetProxyURL(liveProxy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.SetProxyURL("") })
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodHead || r.URL.Host != "probe.example" {
			t.Errorf("unexpected proxy request: %s %s", r.Method, r.URL)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	result, err := testProxy(context.Background(), proxy.URL, "http://probe.example")
	if err != nil || !result.Success || result.Route != "proxy" || result.StatusCode != 204 || requests.Load() != 1 {
		t.Fatalf("draft test = %+v, %v; requests = %d", result, err, requests.Load())
	}
	req, _ := http.NewRequest(http.MethodGet, proxyTestTarget, nil)
	active, _ := http.DefaultTransport.(*http.Transport).Proxy(req)
	if active.String() != liveProxy {
		t.Fatalf("test changed live proxy to %v", active)
	}
	invalid, err := testProxy(context.Background(), "bad proxy", proxyTestTarget)
	if invalid != nil || !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("invalid proxy = %+v, %v", invalid, err)
	}
}

func TestProxyFollowsEnvironment(t *testing.T) {
	if os.Getenv("SLIMEBOT_PROXY_TEST_HELPER") == "1" {
		if err := runtime.SetProxyURL("http://127.0.0.1:12345"); err != nil {
			t.Fatal(err)
		}
		result, err := testProxy(context.Background(), "", "http://probe.example")
		if err != nil || !result.Success || result.Route != "environment" {
			t.Fatalf("environment test = %+v, %v", result, err)
		}
		return
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	// ProxyFromEnvironment caches process variables, so test them in a fresh process.
	command := exec.Command(os.Args[0], "-test.run=^TestProxyFollowsEnvironment$")
	for _, env := range os.Environ() {
		key, _, _ := strings.Cut(env, "=")
		switch strings.ToUpper(key) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "REQUEST_METHOD":
			continue
		}
		command.Env = append(command.Env, env)
	}
	command.Env = append(command.Env, "SLIMEBOT_PROXY_TEST_HELPER=1", "HTTP_PROXY="+proxy.URL)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("environment test failed: %v\n%s", err, output)
	}
}

func TestProxyConnectionDiagnostics(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()
	tlsOrigin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer tlsOrigin.Close()
	authProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("expected CONNECT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer authProxy.Close()
	blockedProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer blockedProxy.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusedProxy := "http://" + listener.Addr().String()
	listener.Close()

	for _, test := range []struct {
		name, proxy, target, route, code string
		status                           int
	}{
		{"direct", "", origin.URL, "direct", "", 204},
		{"loopback bypass", refusedProxy, origin.URL, "direct", "", 204},
		{"refused", refusedProxy, proxyTestTarget, "proxy", "refused", 0},
		{"proxy authentication", authProxy.URL, proxyTestTarget, "proxy", "proxy_auth", 407},
		{"proxy CONNECT failure", blockedProxy.URL, proxyTestTarget, "proxy", "proxy_http", 403},
		{"target HTTP error", blockedProxy.URL, "http://probe.example", "proxy", "http", 403},
		{"certificate", "", tlsOrigin.URL, "direct", "tls", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := testProxy(context.Background(), test.proxy, test.target)
			if err != nil || result.Route != test.route || result.ErrorCode != test.code || result.StatusCode != test.status || result.Success != (test.code == "") {
				t.Fatalf("result = %+v, %v", result, err)
			}
		})
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	result, err := testProxy(ctx, "", origin.URL)
	if err != nil || result.ErrorCode != "timeout" || result.Success {
		t.Fatalf("timeout = %+v, %v", result, err)
	}
	if got := proxyTestErrorCode(&net.DNSError{Err: "no such host", Name: "probe.example"}); got != "dns" {
		t.Fatalf("DNS error code = %q", got)
	}
}

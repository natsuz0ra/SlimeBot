package runtime

import (
	"net/http"
	"testing"
)

func TestSetProxyURL(t *testing.T) {
	t.Cleanup(func() { _ = SetProxyURL("") })
	for _, invalid := range []string{"127.0.0.1:7890", "ftp://127.0.0.1:7890", "http://127.0.0.1", "http://user:pass@127.0.0.1:7890", "http://127.0.0.1:7890/path"} {
		if err := SetProxyURL(invalid); err == nil {
			t.Errorf("accepted invalid proxy URL %q", invalid)
		}
	}
	if err := SetProxyURL("socks5://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport)
	for _, test := range []struct {
		address string
		proxied bool
	}{
		{"https://api.example.com/v1", true},
		{"http://localhost:6247/api", false},
		{"http://127.0.0.1:6247/api", false},
	} {
		req, _ := http.NewRequest(http.MethodGet, test.address, nil)
		proxy, err := transport.Proxy(req)
		if err != nil || (proxy != nil) != test.proxied {
			t.Errorf("proxy(%s) = %v, %v", test.address, proxy, err)
		}
	}
}

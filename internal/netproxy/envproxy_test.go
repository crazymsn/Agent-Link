package netproxy

import (
	"net/url"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/settings"
)

// When the system proxy is off, environment variables must not override it:
// a user who turns the proxy off expects agentlink to go direct, not to
// follow HTTPS_PROXY/HTTP_PROXY/ALL_PROXY set by another tool.
func TestEnvProxyIgnoredWhenSystemProxyIsOff(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
	sysCache.at, sysCache.p = farFuture, Proxy{}
	t.Cleanup(func() { sysCache.at = zero })

	// Set environment proxy but leave system proxy off.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")

	gh, _ := url.Parse("https://github.com/crazymsn/Agent-Link-releases/releases/download/v1/agentlink")
	if p, err := For(gh); err != nil || p != nil {
		t.Fatalf("system proxy off should go direct, got %v %v", p, err)
	}

	// The same with a proxy set in settings still works.
	if err := settings.Save(settings.Settings{Proxy: "http://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	if p, err := For(gh); err != nil || p == nil || p.String() != "http://127.0.0.1:7890" {
		t.Fatalf("settings proxy should be used, got %v %v", p, err)
	}
}

package cli

import "testing"

// agentlink web on loopback behind a reverse proxy (#1479) prints the proxy's
// link with the key when told the proxy's address, so nobody joins
// "/?k=…" onto it by hand: AGENT_LINK_WEB_URL, else AGENT_LINK_PUBLIC_URL with no
// port (one with a port isn't a proxy). None is printed without either.
func TestWebProxyLink(t *testing.T) {
	key := "/?k=0123456789abcdef"
	for _, c := range []struct{ env, want string }{
		{"https://agentlink.example.com", "https://agentlink.example.com/?k=0123456789abcdef"},
		{"https://agentlink.example.com/", "https://agentlink.example.com/?k=0123456789abcdef"},
		{"agentlink.example.com", "http://agentlink.example.com/?k=0123456789abcdef"},
		{"https://example.com/agentlink/", "https://example.com/agentlink/?k=0123456789abcdef"},
		{"http://192.168.1.20:3425", ""},
		{"", ""},
	} {
		t.Setenv("AGENT_LINK_PUBLIC_URL", c.env)
		if got := proxyLink(key); got != c.want {
			t.Errorf("AGENT_LINK_PUBLIC_URL=%q: %q, want %q", c.env, got, c.want)
		}
	}
	// the page's own proxy address, when the gateway's is elsewhere
	t.Setenv("AGENT_LINK_PUBLIC_URL", "https://api.example.com")
	for _, c := range []struct{ env, want string }{
		{"https://agentlink.example.com/", "https://agentlink.example.com/?k=0123456789abcdef"},
		{"agentlink.example.com:8443", "https://agentlink.example.com:8443/?k=0123456789abcdef"},
		{"https://agentlink.example.com/?x=1", ""},
		{"ftp://agentlink.example.com", ""},
	} {
		t.Setenv("AGENT_LINK_WEB_URL", c.env)
		if got := proxyLink(key); got != c.want {
			t.Errorf("AGENT_LINK_WEB_URL=%q: %q, want %q", c.env, got, c.want)
		}
	}
}

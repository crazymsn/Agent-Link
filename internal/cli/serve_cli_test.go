package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/settings"
)

// The serve banner says the gateway takes any key from this machine alone,
// and, when AGENT_LINK_ADDR puts it on the network (a Docker image) or it is
// shared from Settings, a gateway key from others: never from anyone.
func TestKeyNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, c := range []struct{ addr, want string }{
		{"", "only listens on localhost"},
		{"127.0.0.1:3426", "only listens on localhost"},
		{"localhost:3426", "only listens on localhost"},
		{"[::1]:3426", "only listens on localhost"},
		{"0.0.0.0:3425", "from others, an enabled gateway key"},
		{":3425", "from others, an enabled gateway key"},
		{"192.168.1.5:3425", "from others, an enabled gateway key"},
	} {
		t.Setenv("AGENT_LINK_ADDR", c.addr)
		if n := keyNote(); !strings.Contains(n, c.want) || strings.Contains(n, "anyone") {
			t.Errorf("AGENT_LINK_ADDR=%q: %q", c.addr, n)
		}
	}
	for _, key := range []string{"", "sk-agentlink-k"} {
		if err := settings.Save(settings.Settings{LAN: true, LANKey: key}); err != nil {
			t.Fatal(err)
		}
		for _, addr := range []string{"", "0.0.0.0:3425"} {
			t.Setenv("AGENT_LINK_ADDR", addr)
			if n := keyNote(); !strings.Contains(n, "gateway key") || strings.Contains(n, "only listens on localhost") {
				t.Error("shared:", n)
			}
		}
	}
}

// The CLIs print the address other machines are told to reach the gateway
// at: a valid AGENT_LINK_PUBLIC_URL, otherwise the one reached from this
// machine, so `agentlink models` and the serve banner never print a bad one.
func TestAdvertisedURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AGENT_LINK_ADDR", "127.0.0.1:3425")
	for _, c := range []struct{ public, want string }{
		{"", "http://127.0.0.1:3425"},
		{"https://agentlink.example.com", "https://agentlink.example.com"},
		{"nas.lan:3425/", "http://nas.lan:3425"},
		{"ftp://agentlink.example.com", "http://127.0.0.1:3425"},
		{"https://agentlink.example.com/agentlink?x=1", "http://127.0.0.1:3425"},
	} {
		t.Setenv("AGENT_LINK_PUBLIC_URL", c.public)
		if got := advertisedURL(); got != c.want {
			t.Errorf("AGENT_LINK_PUBLIC_URL=%q: %q, want %q", c.public, got, c.want)
		}
	}
}

// Shared, the serve banner (docker logs) says where other machines reach
// the gateway: AGENT_LINK_PUBLIC_URL when a container sets it.
func TestShareLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AGENT_LINK_ADDR", "0.0.0.0:3425")
	t.Setenv("AGENT_LINK_PUBLIC_URL", "http://192.168.1.20:3425/")
	if l := shareLines(); len(l) != 0 {
		t.Error("unshared:", l)
	}
	if err := settings.Save(settings.Settings{LAN: true, LANKey: "sk-agentlink-k"}); err != nil {
		t.Fatal(err)
	}
	if l := strings.Join(shareLines(), "\n"); !strings.Contains(l, "http://192.168.1.20:3425") || strings.Contains(l, "container") {
		t.Error("shared:", l)
	}
}

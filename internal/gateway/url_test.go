package gateway

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/settings"
	"github.com/crazymsn/Agent-Link/internal/testenv"
)

func TestPublicURL(t *testing.T) {
	t.Setenv("AGENT_LINK_ADDR", "0.0.0.0:3499")
	t.Setenv("AGENT_LINK_PUBLIC_URL", "")
	if err := os.Unsetenv("AGENT_LINK_PUBLIC_URL"); err != nil {
		t.Fatal(err)
	}
	if got := PublicURL(); got != "" {
		t.Fatalf("unset: %q, want no public URL", got)
	}
	for _, c := range []struct{ public, want string }{
		{"", ""},
		{"https://agentlink.example.com", "https://agentlink.example.com"},
		{"https://agentlink.example.com/", "https://agentlink.example.com"},
		{"https://agentlink.example.com/agentlink///", "https://agentlink.example.com/agentlink"},
		{"http://agentlink.example.com:3425/", "http://agentlink.example.com:3425"},
		{"http://[::1]:3425/", "http://[::1]:3425"},
		{"agentlink.example.com", "http://agentlink.example.com"},
		{"nas.lan:3425///", "http://nas.lan:3425"},
		{"[fd00::20]:3425", "http://[fd00::20]:3425"},
		{"///", ""},
		{"ftp://agentlink.example.com", ""},
		{"//agentlink.example.com", ""},
		{"https:///agentlink", ""},
		{"http://", ""},
		{"https:///", ""},
		{"ftp://", ""},
		{"https://agentlink.example.com:bad", ""},
		{"https://agentlink.example.com/%zz", ""},
		{" https://agentlink.example.com", "https://agentlink.example.com"},
		{"\thttps://agentlink.example.com/agentlink/ ", "https://agentlink.example.com/agentlink"},
		{"https://agentlink.example.com/agentlink?x=1", ""},
		{"https://agentlink.example.com/agentlink#frag", ""},
		{"https://user:pass@agentlink.example.com", ""},
	} {
		t.Run(c.public, func(t *testing.T) {
			t.Setenv("AGENT_LINK_PUBLIC_URL", c.public)
			if got := PublicURL(); got != c.want {
				t.Errorf("%q, want %q", got, c.want)
			}
			invalid := c.public != "" && c.want == ""
			if invalid {
				if got := publicURL(); got != "" {
					t.Errorf("invalid public URL: %q", got)
				}
				if got := PublicHost(); got != "" {
					t.Errorf("invalid public host: %q", got)
				}
				if got := ContainerAddrs(); got != inContainer("/") {
					t.Errorf("invalid URL must be treated as unset in a container: %v", got)
				}
				for _, got := range LANURLs() {
					if !strings.HasPrefix(got, "http://") {
						t.Errorf("invalid LAN URL: %q", got)
					}
				}
			} else if c.public != "" {
				if got := LANURLs(); len(got) != 1 || got[0] != c.want {
					t.Errorf("LAN URLs: %v, want [%s]", got, c.want)
				}
				if ContainerAddrs() {
					t.Error("valid public URL still says the addresses are a container's")
				}
			}
			if got := Addr(); got != "0.0.0.0:3499" {
				t.Errorf("listen address changed: %q", got)
			}
			if got := URL(); got != "http://127.0.0.1:3499" {
				t.Errorf("local URL changed: %q", got)
			}
		})
	}
}

func TestPublicURLWarningOnce(t *testing.T) {
	// Reset the warning so the assertion does not depend on test order.
	warn := warnInvalidPublicURL
	warnInvalidPublicURL = sync.OnceFunc(logInvalidPublicURL)
	t.Cleanup(func() { warnInvalidPublicURL = warn })
	var logs bytes.Buffer
	writer := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(writer) })
	t.Setenv("AGENT_LINK_ADDR", "127.0.0.1:3499")
	t.Setenv("AGENT_LINK_PUBLIC_URL", "")
	PublicURL()
	if logs.Len() != 0 {
		t.Fatalf("unset URL warned: %q", logs.String())
	}
	t.Setenv("AGENT_LINK_PUBLIC_URL", "nas.lan:3425")
	PublicURL()
	if logs.Len() != 0 {
		t.Fatalf("valid URL warned: %q", logs.String())
	}
	t.Setenv("AGENT_LINK_PUBLIC_URL", "ftp://nas.lan")
	if got := PublicURL(); got != "" {
		t.Fatalf("invalid URL: %q, want no public URL", got)
	}
	if got := strings.Count(logs.String(), "ignoring invalid AGENT_LINK_PUBLIC_URL"); got != 1 {
		t.Fatalf("first invalid URL: %q, want exactly one warning", logs.String())
	}
	logs.Reset()
	for _, u := range []string{"ftp://nas.lan", "https:///agentlink", "https://nas.lan:bad"} {
		t.Setenv("AGENT_LINK_PUBLIC_URL", u)
		for range 3 {
			PublicURL()
			PublicHost()
			LANURLs()
			ContainerAddrs()
		}
	}
	if logs.Len() != 0 {
		t.Fatalf("subsequent calls warned again: %q", logs.String())
	}
}

// OnNetwork: AGENT_LINK_ADDR puts the gateway beyond loopback, shared or not,
// so callers from elsewhere need a gateway key — the console says so, as
// keyNote does.
func TestOnNetwork(t *testing.T) {
	h := t.TempDir()
	testenv.SetHome(t, h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(h, ".local"))
	for _, c := range []struct {
		addr string
		lan  bool
		want bool
	}{
		{"127.0.0.1:3425", false, false},
		{"127.0.0.1:3425", true, false},
		{"localhost:3425", false, false},
		{"[::1]:3425", false, false},
		{"0.0.0.0:3425", false, true},
		{"0.0.0.0:3425", true, true},
		{":3425", false, true},
		{"10.0.0.5:3425", false, true},
		{"10.0.0.5:3425", true, true},
	} {
		t.Setenv("AGENT_LINK_ADDR", c.addr)
		if err := settings.Save(settings.Settings{LAN: c.lan}); err != nil {
			t.Fatal(err)
		}
		if got := OnNetwork(); got != c.want {
			t.Errorf("%s shared=%v: %v, want %v", c.addr, c.lan, got, c.want)
		}
	}
}

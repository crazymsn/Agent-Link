package provider

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/settings"
)

// Agent Link's own gateway is on the port Settings has: an app's entry
// there is Agent Link's, not one to import, and another port is a different
// program once Agent Link has moved off it.
func TestGatewayURLOnSettingsPort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AGENT_LINK_ADDR", "")
	is := func(u string) bool { p, _ := url.Parse(u); return gatewayURL(p) }
	if !is("http://127.0.0.1:7500/v1") || is("http://127.0.0.1:3591/v1") {
		t.Fatal("the default port")
	}
	s := settings.Load()
	s.Port = 3591
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if !is("http://127.0.0.1:3591/v1") || !is("http://localhost:3591") || is("http://127.0.0.1:7500/v1") || is("http://10.0.0.2:3591") {
		t.Fatal("Settings' port")
	}
	t.Setenv("AGENT_LINK_ADDR", "127.0.0.1:3426")
	if !is("http://127.0.0.1:3426") {
		t.Fatal("AGENT_LINK_ADDR's port")
	}
}

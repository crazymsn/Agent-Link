package agent

import (
	"path/filepath"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/edit"
	"github.com/crazymsn/Agent-Link/internal/provider"
	"github.com/crazymsn/Agent-Link/internal/settings"
)

// An agent whose only models through agentlink are routing groups is
// connected on the first of them (#939: the switch said agentlink had no
// models Codex could use while `agentlink codex group/folia-1` connected it).
func TestConnectOnARoutingGroupAlone(t *testing.T) {
	home, _ := codexHome(t, "", "")
	if err := provider.Save(provider.Provider{ID: "aaa", Name: "AAA", Chat: "https://a.example/v1", Key: "k", Models: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(provider.Group{ID: "folia-1", Name: "folia-1", Members: []string{"aaa/m1"}}); err != nil {
		t.Fatal(err)
	}
	s := settings.Load()
	s.HiddenModels = map[string][]string{"claude": {"aaa/m1", "fake/m1"}}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, path, `{}`)
	c := claude(home)
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	if m, _ := edit.GetJSON(path, "model"); m != "group/folia-1" {
		t.Fatalf("connected on %q:\n%s", m, readFile(path))
	}
}

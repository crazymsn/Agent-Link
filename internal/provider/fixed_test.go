package provider

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestAgentLinkProviderIsCanonicalAndFirst(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := Save(Provider{ID: "claude", Name: "Claude", Key: "sk-claude", Chat: "https://claude.example/v1", Models: []string{"m"}}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	p, err := Find(AgentLinkProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != AgentLinkProviderName || p.Chat != AgentLinkProviderURL || p.Icon != AgentLinkProviderIcon {
		t.Fatalf("fixed provider: %#v", p)
	}
	ids := make([]string, 0, len(All()))
	for _, p := range All() {
		ids = append(ids, p.ID)
	}
	if !slices.Equal(ids[:2], []string{AgentLinkProviderID, "claude"}) {
		t.Fatalf("provider order: %v", ids)
	}
}

func TestAgentLinkProviderCannotBeChangedOrRemoved(t *testing.T) {
	isolate(t)
	if err := MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(Provider{ID: AgentLinkProviderID, Name: AgentLinkProviderName, Key: "sk-test", Chat: AgentLinkProviderURL}); err == nil {
		t.Fatal("adding the fixed provider succeeded")
	}
	if err := Save(Provider{ID: AgentLinkProviderID, Name: "Changed", Key: "sk-test", Chat: AgentLinkProviderURL}); err == nil {
		t.Fatal("changing the fixed provider succeeded")
	}
	if err := Delete(AgentLinkProviderID); err == nil {
		t.Fatal("deleting the fixed provider succeeded")
	}
	if err := Rename(AgentLinkProviderID, "changed"); err == nil {
		t.Fatal("renaming the fixed provider succeeded")
	}
	if err := SetOff(AgentLinkProviderID, true); err == nil {
		t.Fatal("turning off the fixed provider succeeded")
	}
}

func TestMigrateFixedProviderRemovesCustomProviders(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := Save(Provider{ID: "custom", Name: "Custom", Key: "sk-custom", Chat: "https://custom.example/v1"}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	if _, err := Find("custom"); err == nil {
		t.Fatal("custom provider survived migration")
	}
	if _, err := Find(AgentLinkProviderID); err != nil {
		t.Fatal(err)
	}
}

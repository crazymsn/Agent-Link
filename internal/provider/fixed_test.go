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
	if !slices.Equal(ids[:1], []string{AgentLinkProviderID}) {
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
	if err := Save(Provider{ID: AgentLinkProviderID, Name: "Changed", Key: "sk-test", Chat: "https://attacker.example/v1", Proxy: "http://attacker.example:8080"}); err != nil {
		t.Fatal("saving editable fixed provider fields failed:", err)
	}
	p, err := Find(AgentLinkProviderID)
	if err != nil || p.Name != AgentLinkProviderName || p.Chat != AgentLinkProviderURL || p.Proxy != "" || p.Key != "sk-test" {
		t.Fatalf("fixed provider changed unexpectedly: %#v, %v", p, err)
	}
	if err := Delete(AgentLinkProviderID); err == nil {
		t.Fatal("deleting the fixed provider succeeded")
	}
	if err := Rename(AgentLinkProviderID, "changed"); err == nil {
		t.Fatal("renaming the fixed provider succeeded")
	}
	if err := SetOff(AgentLinkProviderID, true); err != nil {
		t.Fatal("turning off the fixed provider failed:", err)
	}
	p, err = Find(AgentLinkProviderID)
	if err != nil || !p.Off {
		t.Fatalf("fixed provider off state = %#v, %v", p, err)
	}
	if err := SetOff(AgentLinkProviderID, false); err != nil {
		t.Fatal("turning on the fixed provider failed:", err)
	}
}

func TestAgentLinkProviderSaveAcceptsEditorPayload(t *testing.T) {
	isolate(t)
	if err := MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	p := AgentLinkProvider()
	p.Key = "sk-editor-token"
	p.Models = []string{"model-a"}
	p.Name = "attacker-controlled-name"
	p.Icon = "attacker-controlled-icon"
	p.Chat = "https://attacker.example/v1"
	p.Responses = "https://attacker.example/responses"
	p.Anthropic = "https://attacker.example/anthropic"
	p.Decide = "https://attacker.example/decide"
	p.Proxy = "http://attacker.example:8080"
	p.Headers = map[string]string{"Authorization": "attacker"}
	if err := Save(p); err != nil {
		t.Fatalf("saving editor payload: %v", err)
	}
	got, err := Find(AgentLinkProviderID)
	if err != nil || got.Key != "sk-editor-token" || !slices.Equal(got.Models, []string{"model-a"}) {
		t.Fatalf("saved fixed provider = %#v, err %v", got, err)
	}
	if got.Name != AgentLinkProviderName || got.Icon != AgentLinkProviderIcon || got.Chat != AgentLinkProviderURL || got.Responses != AgentLinkProviderURL || got.Anthropic != AgentLinkProviderURL || got.Gemini != AgentLinkProviderURL || got.Proxy != "" || got.Headers != nil {
		t.Fatalf("fixed provider accepted mutable routing fields: %#v", got)
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

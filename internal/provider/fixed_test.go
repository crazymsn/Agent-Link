package provider

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/catalog"
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
	if p.Name != AgentLinkProviderName || p.Chat != AgentLinkProviderURL || p.Responses != AgentLinkProviderURL || p.Anthropic != "https://api.meding.site" || p.Gemini != AgentLinkProviderURL || p.Icon != AgentLinkProviderIcon || !slices.Equal(p.EnabledProtocols, allProtocols) || p.Base(Chat) != AgentLinkProviderURL || p.Base(Responses) != AgentLinkProviderURL || p.Base(Anthropic) != "https://api.meding.site" || p.Base(Gemini) != AgentLinkProviderURL {
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
	p.PickedFrom = []string{"model-a", "model-old"}
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
	if err != nil || got.Key != "sk-editor-token" || !slices.Equal(got.Models, []string{"model-a"}) || !slices.Equal(got.PickedFrom, []string{"model-a", "model-old"}) {
		t.Fatalf("saved fixed provider = %#v, err %v", got, err)
	}
	if got.Name != AgentLinkProviderName || got.Icon != AgentLinkProviderIcon || got.Chat != AgentLinkProviderURL || got.Responses != AgentLinkProviderURL || got.Anthropic != "https://api.meding.site" || got.Gemini != AgentLinkProviderURL || got.Proxy != "" || got.Headers != nil {
		t.Fatalf("fixed provider accepted mutable routing fields: %#v", got)
	}
}

func TestAgentLinkProviderClearKey(t *testing.T) {
	isolate(t)
	if err := MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	if err := Save(Provider{ID: AgentLinkProviderID, Key: "sk-old"}); err != nil {
		t.Fatal(err)
	}
	if err := AddKey(AgentLinkProviderID, "backup", "sk-backup", ""); err != nil {
		t.Fatal(err)
	}
	p, err := Find(AgentLinkProviderID)
	if err != nil || len(p.KeyList()) != 2 || p.Keys[0].Name != "backup" {
		t.Fatalf("fixed keys = %#v, err %v", p.KeyList(), err)
	}
	if err := RemoveKey(AgentLinkProviderID, KeyID("sk-old")); err != nil {
		t.Fatal(err)
	}
	p, err = Find(AgentLinkProviderID)
	if err != nil || p.Key != "sk-backup" || p.KeyName != "backup" {
		t.Fatalf("fixed primary promotion = %#v, err %v", p, err)
	}
	if err := RemoveKey(AgentLinkProviderID, KeyID("sk-backup")); err != nil {
		t.Fatal(err)
	}
	p, err = Find(AgentLinkProviderID)
	if err != nil || p.Key != "" {
		t.Fatalf("fixed primary key after remove = %q, err %v", p.Key, err)
	}

}
func TestAgentLinkProviderFiltersFetchedModels(t *testing.T) {
	isolate(t)
	if err := MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive(AgentLinkProviderID, AgentLinkProviderURL, []catalog.Model{{ID: "BAAI/bge-m3"}, {ID: "chat-model"}}); err != nil {
		t.Fatal(err)
	}
	p, err := Find(AgentLinkProviderID)
	if err != nil {
		t.Fatal(err)
	}
	ms, _, ok := p.live()
	if !ok || len(ms) != 1 {
		t.Fatalf("unexpected fixture: %#v, %v", ms, ok)
	}
	if got := p.filterFetchedModels(ms); len(got) != 1 || got[0].ID != "chat-model" {
		t.Fatalf("filtered fetched models = %#v", got)
	}
}

func TestAgentLinkProviderFiltersOnlyBlockedModels(t *testing.T) {
	isolate(t)
	if err := MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	fixed := AgentLinkProvider()
	for _, id := range []string{"BAAI/bge-large-en-v1.5", "BAAI/bge-large-zh-v1.5", "BAAI/bge-m3", "BAAI/bge-reranker-v2-m3", "chirp-v3-5"} {
		fixed.Models = append(fixed.Models, id)
	}
	fixed.Models = append(fixed.Models, " chat-model ", "chirp-v3-50")
	if err := Save(fixed); err != nil {
		t.Fatal(err)
	}
	got, err := Find(AgentLinkProviderID)
	if err != nil || !slices.Equal(got.Models, []string{"chat-model", "chirp-v3-50"}) {
		t.Fatalf("fixed filtered models = %v, err %v", got.Models, err)
	}
	other := Provider{ID: "relay", Name: "Relay", Chat: "https://relay.example/v1", Key: "relay-key", Models: []string{"BAAI/bge-m3", "chirp-v3-5"}}
	if err := Save(other); err != nil {
		t.Fatal(err)
	}
	got, err = Find("relay")
	if err != nil || !slices.Equal(got.Models, other.Models) {
		t.Fatalf("ordinary provider models changed = %v, err %v", got.Models, err)
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

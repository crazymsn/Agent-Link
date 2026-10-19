package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/provider"
)

// A remote agentlink is added from the TUI with its address, as the app's
// editor and agentlink provider add remote-agentlink … url=… add it, and its
// address is changed there later (akic404 on Discord: the TUI had no place
// for it, only the key).
func TestTUIAddsARemoteAgentLinkWithItsAddress(t *testing.T) {
	home(t)
	m := press(t, model{w: 120, h: 40}, "2", "a")
	if m.mode != modePick {
		t.Fatalf("a opened mode %v, not the vendors", m.mode)
	}
	m = typeIn(m, "remote-agentlink")
	m = press(t, m, "enter")
	if m.mode != modeAsk || m.ask.input.Placeholder != "http://192.168.1.20:3425" {
		t.Fatalf("picking Remote agentlink asked %q (mode %v), not its address", m.ask.input.Placeholder, m.mode)
	}
	// nothing typed: said to be needed, nothing added
	m = press(t, m, "enter")
	wantFlash(t, m, false, "The other agentlink's address is needed")
	if _, err := provider.Find("remote-agentlink"); err == nil {
		t.Fatal("added without an address")
	}

	m = press(t, m, "a")
	m = typeIn(m, "remote-agentlink")
	m = press(t, m, "enter")
	m = typeIn(m, "127.0.0.1:1")
	m = press(t, m, "enter")
	if m.mode != modeAsk || m.ask.input.Placeholder != "the API key" {
		t.Fatalf("after the address it asked %q (mode %v), not the key", m.ask.input.Placeholder, m.mode)
	}
	m = typeIn(m, "sk-agentlink-test")
	m = press(t, m, "enter")
	// nothing answers there: added, and said why it has no models
	wantFlash(t, m, false, "added Remote agentlink · no model list")
	p, err := provider.Find("remote-agentlink")
	if err != nil {
		t.Fatal(err)
	}
	if p.Chat != "http://127.0.0.1:1/v1" || p.Responses != "http://127.0.0.1:1/v1" || p.Anthropic != "http://127.0.0.1:1" || p.Key != "sk-agentlink-test" {
		t.Fatalf("saved %q %q %q key %q", p.Chat, p.Responses, p.Anthropic, p.Key)
	}

	// w changes its address, the one there now in the line
	m.reloadProviders()
	for i, q := range m.provs {
		if q.ID == "remote-agentlink" {
			m.prow = i
		}
	}
	m = press(t, m, "w")
	if m.mode != modeAsk || m.ask.input.Value() != "http://127.0.0.1:1" {
		t.Fatalf("w opened %q (mode %v), not the address now", m.ask.input.Value(), m.mode)
	}
	m = press(t, m, "ctrl+u")
	m = typeIn(m, "http://127.0.0.1:2/v1")
	m = press(t, m, "enter")
	wantFlash(t, m, true, "address")
	p, _ = provider.Find("remote-agentlink")
	if p.Chat != "http://127.0.0.1:2/v1" || p.Anthropic != "http://127.0.0.1:2" || p.Key != "sk-agentlink-test" {
		t.Fatalf("after w: %q %q key %q", p.Chat, p.Anthropic, p.Key)
	}

	// a vendor of its own address has none to change (a provider of the
	// user's own does: TestTUIEditsACustomProvidersBaseURL)
	if err := provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Preset: "deepseek", Key: "kd", Chat: "https://api.deepseek.com/v1"}); err != nil {
		t.Fatal(err)
	}
	m.reloadProviders()
	for i, q := range m.provs {
		if q.ID == "deepseek" {
			m.prow = i
		}
	}
	m = press(t, m, "w")
	if m.mode == modeAsk {
		t.Fatal("w asked a vendor's address")
	}
}

// Adding a provider fetches its list and says what came of it — how many
// models, or why none — and m on the Providers page fetches it again, as
// the app editor's Fetch models does (akic404 on Discord: added from the TUI,
// a remote agentlink had 0 models, with nothing said and no way to fetch
// them).
// The add list says what Remote agentlink is, another computer's agentlink, and
// doesn't file it under the relays (Avalon on Discord: "供应商添加中没有
// Remote agentlink 的选项").
func TestTUIAddListSaysRemoteAgentLinkIsAnotherComputers(t *testing.T) {
	home(t)
	m := press(t, model{w: 120, h: 40}, "2", "a")
	for _, o := range m.pk.items {
		if o.Value != provider.RemoteAgentLinkPreset {
			continue
		}
		if o.Note != "Remote agentlink · another computer's agentlink, shared on its network" {
			t.Fatalf("Remote agentlink is listed as %q", o.Note)
		}
		return
	}
	t.Fatal("Remote agentlink is not in the add list")
}

func TestTUIFetchesAProvidersModels(t *testing.T) {
	home(t)
	var mu sync.Mutex
	ids := []string{"custom/qwen3.6-max", "custom/glm-5.3"}
	down := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if down {
			http.Error(w, `{"error":"invalid gateway key"}`, http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		var data []map[string]any
		for _, id := range ids {
			data = append(data, map[string]any{"id": id, "object": "model", "display_name": id, "agentlink_label": id + " · 中转站"})
		}
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	}))
	defer srv.Close()
	set := func(d bool, list ...string) {
		mu.Lock()
		defer mu.Unlock()
		down = d
		if list != nil {
			ids = list
		}
	}
	at := func(m model, id string) model {
		m.reloadProviders()
		for i, q := range m.provs {
			if q.ID == id {
				m.prow = i
			}
		}
		return m
	}

	m := press(t, model{w: 160, h: 40}, "2", "a")
	m = typeIn(m, "remote-agentlink")
	m = press(t, m, "enter")
	m = typeIn(m, strings.TrimPrefix(srv.URL, "http://"))
	m = press(t, m, "enter")
	m = typeIn(m, "sk-agentlink-test")
	m = press(t, m, "enter")
	wantFlash(t, m, true, "added Remote agentlink · 2 models from its list · 2 for agents")
	p, err := provider.Find("remote-agentlink")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range p.Exposed() {
		got = append(got, x.ID)
	}
	if strings.Join(got, ",") != "custom/qwen3.6-max,custom/glm-5.3" {
		t.Fatalf("exposed %v", got)
	}

	// the other agentlink's list grows: m fetches it again
	set(false, "custom/qwen3.6-max", "custom/glm-5.3", "custom/kimi-k3")
	m = at(m, "remote-agentlink")
	m = press(t, m, "m")
	wantFlash(t, m, true, "Remote agentlink: 3 models from its list · 3 for agents")
	if !strings.Contains(m.View(), "3 models") {
		t.Fatalf("the row doesn't say 3 models after m:\n%s", m.View())
	}

	// refused: said so, and the list had stays
	set(true)
	m = press(t, m, "m")
	wantFlash(t, m, false, "no model list")
	wantFlash(t, m, false, "m asks again")
	if p, _ := provider.Find("remote-agentlink"); len(p.Exposed()) != 3 {
		t.Fatalf("a failed fetch left %d models", len(p.Exposed()))
	}

	// from the provider's models, where a free list is looked at, ctrl+r
	// fetches it again and the list open shows it (#2113)
	set(false, "custom/qwen3.6-max", "custom/glm-5.3", "custom/kimi-k3", "custom/ling-3.1-flash-free")
	m = at(m, "remote-agentlink")
	m = press(t, m, "enter")
	if m.mode != modePick || !strings.Contains(m.View(), "ctrl+r") {
		t.Fatalf("the models view doesn't offer ctrl+r:\n%s", m.View())
	}
	if strings.Contains(m.View(), "ling-3.1-flash-free") {
		t.Fatalf("listed before it was fetched:\n%s", m.View())
	}
	m = press(t, m, "ctrl+r")
	wantFlash(t, m, true, "Remote agentlink: 4 models from its list")
	if m.mode != modePick || !strings.Contains(m.View(), "custom/ling-3.1-flash-free") {
		t.Fatalf("the models view doesn't list what ctrl+r fetched:\n%s", m.View())
	}
	// a key typed is still the filter's
	m = typeIn(m, "r")
	if m.pk.input.Value() != "r" {
		t.Fatalf("filter %q", m.pk.input.Value())
	}
}

package gateway

import (
	"testing"

	"github.com/crazymsn/Agent-Link/internal/provider"
	"github.com/crazymsn/Agent-Link/internal/settings"
)

// With provider search Off, this agentlink's model list no longer tells
// another agentlink that a Kimi Code plan's models are searched for by it:
// without a search API it adds no search tool to them (#1483).
func TestSearchProviderOffTellsRemoteAgentLink(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	setHome(t, t.TempDir())
	kimi := newKimiPlan(t)
	p := provider.Provider{ID: "kimi", Name: "Kimi Code", Preset: "kimi-code-cn", Key: "sk-kimi",
		Chat: kimi.URL + "/coding/v1", Models: []string{"kimi-for-coding"}}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	e := provider.Entry{Provider: p, Model: "kimi-for-coding"}
	if got := webSearchOf(e, canSearch()); got != searchAgentLink {
		t.Fatalf("automatic: %q, want %q", got, searchAgentLink)
	}
	st := settings.Load()
	st.Searcher = "off"
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if got := webSearchOf(e, canSearch()); got != "" {
		t.Errorf("off, no search API: %q, want none", got)
	}
}

package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/provider"
)

// remoteList is the list another agentlink (v0.1 on main, its own Provider in
// model names on) answered this one's remote-agentlink with, as served on
// 127.0.0.1:3572 in a sandbox: its group and its relay's two models, each
// with display_name its name alone and agentlink_label its name with its
// provider there after it.
const remoteList = `{"data":[{"context_length":202752,"context_window":202752,"created":0,"created_at":"2025-01-01T00:00:00Z","display_name":"Fast","id":"group/fast","agentlink_label":"Fast · routing group","max_input_tokens":202752,"max_output_tokens":202752,"object":"model","owned_by":"relaya","reasoning":true,"supported_reasoning_levels":[],"type":"model"},{"context_length":128000,"context_window":128000,"created":0,"created_at":"2025-01-01T00:00:00Z","display_name":"DeepSeek Chat","id":"relaya/deepseek-chat","agentlink_label":"DeepSeek Chat · Relay A","max_input_tokens":128000,"max_output_tokens":128000,"native_endpoints":["/v1/chat/completions"],"object":"model","owned_by":"relaya","reasoning":false,"supported_reasoning_levels":[],"type":"model"},{"context_length":202752,"context_window":202752,"created":0,"created_at":"2025-01-01T00:00:00Z","display_name":"GLM-5","id":"relaya/glm-5","agentlink_label":"GLM-5 · Relay A","max_input_tokens":202752,"max_output_tokens":131072,"native_endpoints":["/v1/chat/completions"],"object":"model","owned_by":"relaya","reasoning":true,"supported_reasoning_levels":[],"type":"model"}],"first_id":"group/fast","has_more":false,"last_id":"relaya/glm-5","object":"list"}`

// Provider in model names Off names a remote agentlink's models alone here
// too (ARNO on Discord): its list's labels carry the other agentlink's
// providers after them by that agentlink's own setting, and they were kept
// whatever this one's said — "DeepSeek Chat · Relay A", "Fast · routing
// group" under Off. On, they keep that provider and this one's after it,
// as before; a name the user gave one here is theirs under own.
func TestRemoteAgentLinkSuffixOff(t *testing.T) {
	fresh(t)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, remoteList)
	}))
	t.Cleanup(remote.Close)
	id, err := provider.Add(provider.Provider{ID: "remote-agentlink", Name: "Remote agentlink", Key: "agentlink", Preset: provider.RemoteAgentLinkPreset, Chat: remote.URL})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := provider.Find(id)
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetModelName("remote-agentlink/relaya/glm-5", "My GLM"); err != nil {
		t.Fatal(err)
	}
	list := func() map[string]string {
		rec := httptest.NewRecorder()
		New().Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/v1/models", nil))
		var out struct {
			Data []struct {
				ID    string `json:"id"`
				Name  string `json:"display_name"`
				Label string `json:"agentlink_label"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, m := range out.Data {
			got[strings.TrimPrefix(m.ID, "remote-agentlink/")] = m.Name + "|" + m.Label
		}
		return got
	}
	for _, c := range []struct{ mode, group, ds, glm string }{
		{provider.SuffixOn, "Fast|Fast · routing group · Remote agentlink", "DeepSeek Chat|DeepSeek Chat · Relay A · Remote agentlink", "My GLM|My GLM · Remote agentlink"},
		{provider.SuffixOwn, "Fast|Fast · routing group · Remote agentlink", "DeepSeek Chat|DeepSeek Chat · Relay A · Remote agentlink", "My GLM|My GLM"},
		{provider.SuffixOff, "Fast|Fast", "DeepSeek Chat|DeepSeek Chat", "My GLM|My GLM"},
	} {
		if err := provider.SetSuffixMode(c.mode); err != nil {
			t.Fatal(err)
		}
		got := list()
		if got["group/fast"] != c.group || got["relaya/deepseek-chat"] != c.ds || got["relaya/glm-5"] != c.glm {
			t.Errorf("%s: %v", c.mode, got)
		}
	}
}

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crazymsn/Agent-Link/internal/edit"
	"github.com/crazymsn/Agent-Link/internal/provider"
)

// airHome is a sandbox home with Air's config folder and an OpenCode at a
// known place.
func airHome(t *testing.T, bin string) (home, dir string) {
	home, _ = codexHome(t, "", "")
	dir = airDir(home, filepath.Join(home, ".config"))
	was := airOpenCode
	airOpenCode = func(string) string { return bin }
	t.Cleanup(func() { airOpenCode = was })
	return home, dir
}

// Air's model menu is the ACP agent's: agentlink adds one, AgentLink, to
// acp.json beside the user's own agents — OpenCode's ACP server on a config
// of agentlink's that has agentlink's provider alone — and taking it off takes
// out that entry and that file, the user's agents kept.
func TestAirAddsAgentLinkAgent(t *testing.T) {
	home, dir := airHome(t, "/opt/bin/opencode")
	acp := filepath.Join(dir, "acp.json")
	writeFile(t, acp, `{
    "agent_servers": {
        "Goose": {"command": "goose", "args": ["acp"], "env": {}}
    }
}
`)
	a := air(home, filepath.Join(home, ".config"))
	if !a.Detected() || a.Path != acp {
		t.Fatalf("detected %v, path %s", a.Detected(), a.Path)
	}
	if v := a.Field("model").Get(); v != "" {
		t.Fatalf("model before = %q", v)
	}
	opts := a.Field("model").Options(nil)
	want := ""
	for _, o := range opts {
		if o.Ref == "fake/m1" {
			want = o.Value
		}
	}
	if want != "agentlink/fake/m1" {
		t.Fatalf("options: %+v", opts)
	}
	if err := a.Apply("model", want); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "agentlink-opencode.json")
	for k, v := range map[string]string{
		"agent_servers.Goose.command":              "goose",
		"agent_servers.AgentLink.command":             "/opt/bin/opencode",
		"agent_servers.AgentLink.args.0":              "acp",
		"agent_servers.AgentLink.env.OPENCODE_CONFIG": cfg,
	} {
		if got, _ := edit.GetJSON(acp, k); got != v {
			t.Fatalf("%s = %q, want %q\n%s", k, got, v, readFile(acp))
		}
	}
	for k, v := range map[string]string{
		"model":                           "agentlink/fake/m1",
		"small_model":                     "agentlink/fake/m1",
		"enabled_providers":               `["agentlink"]`,
		"provider.agentlink.options.baseURL": gatewayV1(),
		"provider.agentlink.options.apiKey":  "agentlink",
		"provider.agentlink.npm":             "@ai-sdk/openai-compatible",
	} {
		if got, _ := edit.GetJSON(cfg, k); strings.Join(strings.Fields(got), "") != v {
			t.Fatalf("%s = %q, want %q\n%s", k, got, v, readFile(cfg))
		}
	}
	if _, ok := edit.GetJSON(cfg, `provider.agentlink.models.fake/m1`); !ok {
		t.Fatalf("no fake/m1 in agentlink's models: %s", readFile(cfg))
	}
	if v := a.Field("model").Get(); v != want {
		t.Fatalf("model = %q", v)
	}
	if d := a.Drift(); d != nil {
		t.Fatalf("drift: %+v", d)
	}
	if n := a.Notice(); !strings.Contains(n, "pick AgentLink") {
		t.Fatalf("notice: %q", n)
	}
	// one of Air's own agents isn't set from here
	if err := a.Apply("model", "anthropic/claude-sonnet-5"); err == nil {
		t.Fatal("took a model that isn't agentlink's")
	}
	if err := a.Apply("model", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := edit.GetJSON(acp, "agent_servers.AgentLink"); ok {
		t.Fatalf("AgentLink still in acp.json: %s", readFile(acp))
	}
	if v, _ := edit.GetJSON(acp, "agent_servers.Goose.command"); v != "goose" {
		t.Fatalf("the user's agent went: %s", readFile(acp))
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Fatalf("agentlink's OpenCode config left: %v", err)
	}
	if v := a.Field("model").Get(); v != "" {
		t.Fatalf("model after = %q", v)
	}
}

// With no acp.json yet (Air makes it on the first "Edit ACP JSON File"),
// agentlink writes one; without OpenCode found it names it by its command and
// the notice says to install it.
func TestAirWithoutOpenCode(t *testing.T) {
	home, dir := airHome(t, "")
	os.MkdirAll(dir, 0o755)
	a := air(home, filepath.Join(home, ".config"))
	if err := a.Apply("model", "agentlink/fake/m1"); err != nil {
		t.Fatal(err)
	}
	acp := filepath.Join(dir, "acp.json")
	if v, _ := edit.GetJSON(acp, "agent_servers.AgentLink.command"); v != "opencode" {
		t.Fatalf("command = %q\n%s", v, readFile(acp))
	}
	if n := a.Notice(); !strings.Contains(n, "install OpenCode") {
		t.Fatalf("notice: %q", n)
	}
}

// AgentLink's entry taken out of acp.json by hand while its config still
// names one of agentlink's models is wiring gone, which setting it again puts
// back; providers added later reach the config's model list, and Sync
// writes no config agentlink never set up.
func TestAirCheckAndSync(t *testing.T) {
	home, dir := airHome(t, "/opt/bin/opencode")
	a := air(home, filepath.Join(home, ".config"))
	acp, cfg := filepath.Join(dir, "acp.json"), filepath.Join(dir, "agentlink-opencode.json")
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Fatal("Sync wrote a config agentlink never set up")
	}
	if err := a.Apply("model", "agentlink/fake/m1"); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: "fake2", Name: "Fake 2", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, ok := edit.GetJSON(cfg, `provider.agentlink.models.fake2/m2`); !ok {
		t.Fatalf("a provider added later isn't listed: %s", readFile(cfg))
	}
	if err := edit.DelJSON(acp, "agent_servers.AgentLink"); err != nil {
		t.Fatal(err)
	}
	if d := a.Drift(); d == nil || d.Kind != "unwired" || !strings.Contains(d.Detail, "AgentLink agent") {
		t.Fatalf("drift: %+v", d)
	}
	if err := a.Reapply(); err != nil {
		t.Fatal(err)
	}
	if v, _ := edit.GetJSON(acp, "agent_servers.AgentLink.env.OPENCODE_CONFIG"); v != cfg {
		t.Fatalf("not put back: %s", readFile(acp))
	}
	if d := a.Drift(); d != nil {
		t.Fatalf("drift after setting it again: %+v", d)
	}
}

// Sync and a model pick keep the user's provider fields, while agentlink's
// endpoint, key and complete model list still follow the gateway.
func TestAirProviderWritesKeepCustomFields(t *testing.T) {
	for _, action := range []string{"sync", "pick"} {
		t.Run(action, func(t *testing.T) {
			home, dir := airHome(t, "/opt/bin/opencode")
			a := air(home, filepath.Join(home, ".config"))
			cfg := filepath.Join(dir, "agentlink-opencode.json")
			if err := a.Apply("model", "agentlink/fake/m1"); err != nil {
				t.Fatal(err)
			}
			writeFile(t, cfg, `{
  // the user's configuration
  "theme": "dark",
  "model": "agentlink/fake/m1",
  "provider": {
    "mine": {"name": "mine"},
    "agentlink": {
      "name": "old",
      "npm": "old",
      "options": {
        "baseURL": "http://127.0.0.1:1/v1",
        "apiKey": "old-key",
        "timeout": 12345,
        "headers": {"z": "last", "a": "first"}
      },
      "compat": {"sendSessionAffinityHeaders": true, "supportsLongCacheRetention": false},
      "marker": {"z": 9007199254740993, "a": null},
      "models": {"fake/m1": {"obsolete": true}, "removed/m0": {}}
    }
  }
}
`)
			kept := map[string]string{}
			for _, field := range []string{"compat", "marker", "options.timeout", "options.headers"} {
				kept[field], _ = edit.GetJSON(cfg, "provider.agentlink."+field)
			}
			check := func() {
				t.Helper()
				for field, was := range kept {
					if v, ok := edit.GetJSON(cfg, "provider.agentlink."+field); !ok || !sameOrder(v, json.RawMessage(was)) {
						t.Errorf("custom %s lost or changed: got %q, want %q", field, v, was)
					}
				}
				if !strings.Contains(readFile(cfg), "// the user's configuration") {
					t.Error("the user's comment was removed")
				}
				for k, want := range map[string]string{"theme": "dark", "provider.mine.name": "mine"} {
					if got, _ := edit.GetJSON(cfg, k); got != want {
						t.Errorf("%s = %q, want %q", k, got, want)
					}
				}
				want := agentlinkProviderJSONFor("opencode", "air").(map[string]any)
				for k, v := range want["options"].(map[string]any) {
					if got, _ := edit.GetJSON(cfg, "provider.agentlink.options."+k); got != v {
						t.Errorf("options.%s = %q, want %v", k, got, v)
					}
				}
				for _, k := range []string{"name", "npm", "models"} {
					got, _ := edit.GetJSON(cfg, "provider.agentlink."+k)
					if k != "models" {
						b, _ := json.Marshal(got)
						got = string(b)
					}
					if !sameOrder(got, want[k]) {
						t.Errorf("managed %s is stale: %s", k, got)
					}
				}
			}
			if action == "sync" {
				if err := a.Sync(); err != nil {
					t.Fatal(err)
				}
			} else if err := a.Apply("model", "agentlink/fake/m1"); err != nil {
				t.Fatal(err)
			}
			check()
			// The old catalog entry must go when the provider changes.
			if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}}); err != nil {
				t.Fatal(err)
			}
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			check()
			before := readFile(cfg)
			stamp := time.Unix(1000000000, 0)
			if err := os.Chtimes(cfg, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			stat, err := os.Stat(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if readFile(cfg) != before || !stat.ModTime().Equal(stamp) {
				t.Error("unchanged provider was rewritten")
			}
		})
	}
}

// An OpenCode a JetBrains IDE or Zed installed from the ACP registry, in
// the editor's own folder and on no PATH, is the one Air starts (#2423,
// whiteS18: IDEA 2026.2's on macOS); the newest of several.
func TestAirFindsACPRegistryOpenCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	put := func(p string, age time.Duration) string {
		writeFile(t, p, "#!/bin/sh\n")
		when := time.Now().Add(-age)
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, c := range []struct {
		goos, name  string
		older, want string
	}{
		{"darwin", "opencode",
			filepath.Join(home, "Library", "Application Support", "Zed", "external_agents", "registry", "opencode", "v_1.18.11_c8581a12e4e7afe7_fd7ca24743b4ae6b", "opencode"),
			filepath.Join(home, "Library", "Caches", "JetBrains", "IntelliJIdea2026.2", "acp-agents", "opencode", "1.19.0", "opencode")},
		{"linux", "opencode",
			filepath.Join(home, ".local", "share", "zed", "external_agents", "registry", "opencode", "v_1.18.11", "opencode"),
			filepath.Join(home, ".cache", "JetBrains", "IntelliJIdea2026.2", "acp-agents", "opencode", "1.19.0", "opencode")},
		{"windows", "opencode.exe",
			filepath.Join(home, "AppData", "Local", "JetBrains", "IntelliJIdea2026.2", "acp-agents", "opencode", "1.18.0", "opencode.exe"),
			filepath.Join(home, "AppData", "Local", "Zed", "external_agents", "registry", "opencode", "v_1.19.0", "opencode.exe")},
	} {
		if got := acpOpenCode(home, c.name, c.goos); got != "" {
			t.Fatalf("%s: none installed, got %q", c.goos, got)
		}
		put(c.older, time.Hour)
		put(c.want, time.Minute)
		if got := acpOpenCode(home, c.name, c.goos); got != c.want {
			t.Fatalf("%s: got %q, want %q", c.goos, got, c.want)
		}
	}
}

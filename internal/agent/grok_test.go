package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/edit"
	"github.com/crazymsn/Agent-Link/internal/plugin"
	"github.com/crazymsn/Agent-Link/internal/provider"
	"github.com/crazymsn/Agent-Link/internal/settings"
)

func TestGrok(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("GROK_HOME", filepath.Join(home, "grok"))
	if err := provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k", Models: []string{"pro", "flash"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "grok", "config.toml")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("# mine\n[ui]\ntheme = \"dark\"\n\n[model.my-own]\nmodel = \"x\"\nbase_url = \"https://x/v1\"\n\n[models]\ndefault = \"grok-4.6\"\n"), 0o644)
	read := func() string { b, _ := os.ReadFile(path); return string(b) }
	table := func(name string) map[string]string {
		t.Helper()
		got, err := edit.GetTOMLTable(path, name)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	a := grok(home)
	if a.Path != path {
		t.Fatalf("path: %s", a.Path)
	}
	f := a.Field("model")
	if f.Get() != "grok-4.6" {
		t.Fatalf("get: %q", f.Get())
	}

	if err := f.Set("agentlink/deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	raw := read()
	m := table(`model."agentlink/deepseek/pro"`)
	if m["model"] != "deepseek/pro" || m["api_key"] != "agentlink" || !strings.HasSuffix(m["base_url"], "/v1") ||
		m["api_backend"] != "chat_completions" || table(`model."agentlink/deepseek/flash"`) == nil {
		t.Fatalf("tables:\n%s", raw)
	}
	// a campaign of xAI's would set the default over it
	if table("features")["campaigns"] != "false" {
		t.Fatalf("campaigns:\n%s", raw)
	}
	if f.Get() != "agentlink/deepseek/pro" || !strings.Contains(raw, "# mine") || !strings.Contains(raw, "[model.my-own]") ||
		table("ui")["theme"] != "dark" {
		t.Fatalf("config:\n%s", raw)
	}

	// picked again, and synced: the tables are replaced, never repeated
	if err := f.Set("agentlink/deepseek/flash"); err != nil {
		t.Fatal(err)
	}
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	raw = read()
	if n := strings.Count(raw, `[model."agentlink/deepseek/pro"]`); n != 1 {
		t.Fatalf("%d tables:\n%s", n, raw)
	}

	// a provider removed leaves Grok's list on the next sync
	provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k", Models: []string{"flash"}})
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(), `"agentlink/deepseek/pro"]`) {
		t.Fatalf("stale:\n%s", read())
	}

	// its own model: agentlink steps out, the user's own model stays
	if err := f.Set("grok-4.6"); err != nil {
		t.Fatal(err)
	}
	raw = read()
	if f.Get() != "grok-4.6" || strings.Contains(raw, "agentlink") || !strings.Contains(raw, "[model.my-own]") {
		t.Fatalf("own:\n%s", raw)
	}
	// nothing through agentlink: sync leaves the file alone
	if err := a.Sync(); err != nil || read() != raw {
		t.Fatalf("sync touched it:\n%s", read())
	}

	e := a.Field("effort")
	f.Set("agentlink/deepseek/flash")
	if err := e.Set("high"); err != nil || e.Get() != "high" {
		t.Fatalf("effort: %v %q", err, e.Get())
	}
	if err := f.Set(""); err != nil {
		t.Fatal(err)
	}
	raw = read()
	if f.Get() != "" || strings.Contains(raw, "agentlink") || strings.Contains(raw, "campaigns") || e.Get() != "high" {
		t.Fatalf("reset:\n%s", raw)
	}
}

func TestGrokReadErrorStopsChanges(t *testing.T) {
	const wired = "[model.\"agentlink/keep\"]\nmodel = \"keep\"\n\n"
	for _, tc := range []struct {
		name, input string
	}{
		{"models", wired + "[models]\ndefault = [\n"},
		{"features", "[models]\ndefault = \"agentlink/keep\"\n\n" + wired + "[features]\ncampaigns = [\n"},
	} {
		for _, action := range []string{"set", "clear", "sync"} {
			t.Run(tc.name+"/"+action, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("GROK_HOME", home)
				path := filepath.Join(home, "config.toml")
				if err := os.WriteFile(path, []byte(tc.input), 0o600); err != nil {
					t.Fatal(err)
				}
				a := grok(home)
				var err error
				switch action {
				case "set":
					err = a.Field("model").Set("grok-native")
				case "clear":
					err = a.Field("model").Set("")
				case "sync":
					err = a.Sync()
				}
				if err == nil || !strings.HasPrefix(err.Error(), path+": ") {
					t.Fatalf("expected the config path in the error, got %v", err)
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != tc.input {
					t.Fatalf("changed config after a read error:\n%s", got)
				}
			})
		}
	}
}

func TestGrokSyncRequiresOrdinaryModelTable(t *testing.T) {
	for _, body := range []string{
		"note = '''\n[model.\"agentlink/fake\"]\n'''\n",
		"[[model.\"agentlink/array\"]]\nmodel = \"array\"\n",
	} {
		t.Run(body, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("GROK_HOME", home)
			path := filepath.Join(home, "config.toml")
			input := "[models]\ndefault = \"grok-native\"\n" + body
			if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := grok(home).Sync(); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != input {
				t.Fatalf("synced a config without a agentlink model table:\n%s", got)
			}
		})
	}
}

func TestGrokCheckReportsTOMLParseErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GROK_HOME", home)
	path := filepath.Join(home, "config.toml")
	input := "[models]\ndefault = \"agentlink/fake\"\n\n[model.\"agentlink/fake\"]\ninvalid = [\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	message := grok(home).Check()
	if !strings.HasPrefix(message, path+": line 5, column 11:") || !strings.Contains(message, "array is incomplete") {
		t.Fatalf("check did not report the parse location: %q", message)
	}
}

// The models agentlink serves on the account Grok Build is signed in to are
// Grok's own a second time, beside its own: they fold into one row of the
// picker (Fate on Discord: grokbuild 在登录态下会加载重复的模型), the
// other providers' rows left as they are.
func TestGrokFoldsItsOwnAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("GROK_HOME", filepath.Join(home, "grok"))
	exe := provider.GrokExecutable
	provider.GrokExecutable = func() string { return filepath.Join(home, "grok", "bin", "grok") }
	t.Cleanup(func() { provider.GrokExecutable = exe })
	provider.ForgetAccounts()
	t.Cleanup(provider.ForgetAccounts)
	os.MkdirAll(filepath.Join(home, "grok"), 0o755)
	os.WriteFile(filepath.Join(home, "grok", "auth.json"), []byte(`{"x":{"key":"k","email":"me@x.ai"}}`), 0o600)
	// the Grok plugin serves the account, taken from Grok Build's own
	spec := "@agentlink-community/opencode-grok-auth"
	write := func(name string, v any) {
		t.Helper()
		b, _ := json.Marshal(v)
		if err := os.MkdirAll(settings.Dir(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(settings.Dir(), name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("plugins.json", map[string]any{"plugins": []map[string]any{{"spec": spec}}})
	write("plugin-auth.json", map[string]map[string]any{"grok": {"type": "oauth", "refresh": filepath.Join(home, "grok")}})
	plugin.UseCached([]plugin.Provider{{ID: "grok", Spec: spec, Name: "Grok",
		Models: []plugin.Model{{ID: "grok-4.3", Name: "Grok 4.3"}}, Accounts: []plugin.Account{{Key: "grok", Type: "oauth"}}}})
	t.Cleanup(plugin.Settle)
	if err := provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k", Models: []string{"pro"}}); err != nil {
		t.Fatal(err)
	}
	a := grok(home)
	var same, others []string
	for _, o := range a.Field("model").Options(a.Values()) {
		if o.Same {
			same = append(same, o.Value)
		} else if strings.HasPrefix(o.Value, "agentlink/") {
			others = append(others, o.Value)
		}
	}
	if len(same) == 0 {
		t.Fatalf("nothing folded; agentlink's rows: %v", others)
	}
	for _, v := range same {
		if !strings.HasPrefix(v, "agentlink/grok/") {
			t.Errorf("folded %s, not on the Grok account", v)
		}
	}
	for _, v := range others {
		if strings.HasPrefix(v, "agentlink/grok/") {
			t.Errorf("%s on the Grok account not folded", v)
		}
	}
	if !slices.Contains(others, "agentlink/deepseek/pro") {
		t.Errorf("DeepSeek's row: %v", others)
	}
}

package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/edit"
	"github.com/crazymsn/Agent-Link/internal/provider"
)

// wevnal is the #2055 reporter's provider, its models the ones Pi warned
// about once deselected, beside two that stay.
func wevnalSave(t *testing.T, models ...string) {
	t.Helper()
	if err := provider.Save(provider.Provider{ID: "wevnal", Name: "Wevnal", Chat: "https://wevnal.invalid/v1", Key: "k", Models: models}); err != nil {
		t.Fatal(err)
	}
}

var wevnalAll = []string{"glm-5.2", "kimi-k3", "mimo-v2.6-flash-free", "muse-spark-1.3-contributor-free", "nemotron-3.5-lightning-free", "space-bunny-free"}

func piList(t *testing.T, settings string) string {
	t.Helper()
	raw, ok := edit.GetJSON(settings, "enabledModels")
	if !ok {
		return "(none)"
	}
	return strings.Join(strings.Fields(raw), "")
}

// #2055 (misaka9981): models deselected in a provider's editor leave
// agentlink's provider in Pi's models.json, and the entries naming them in
// enabledModels go too, in Pi and the forks that keep its settings.json:
// Pi warned "No models match pattern" for each. The user's other
// entries, a glob on agentlink, entries on a model still there (one at a
// thinking level), their order and comments stay. The ones taken out are
// remembered: selected again, a model's entry is back, and a rename takes
// a remembered one with it.
func TestPiEnabledModelsLoseDeselected(t *testing.T) {
	for _, c := range []struct {
		id  string
		dir []string
		a   func(string) *Agent
	}{
		{"pi", []string{".pi", "agent"}, pi},
		{"omo", []string{".omo", "agent"}, omo},
		{"prime-agent", []string{".prime", "agent"}, primeAgent},
	} {
		t.Run(c.id, func(t *testing.T) {
			home := syncHome(t)
			wevnalSave(t, wevnalAll...)
			settings := filepath.Join(append([]string{home}, append(c.dir, "settings.json")...)...)
			os.MkdirAll(filepath.Dir(settings), 0o755)
			os.WriteFile(settings, []byte(`{
  // the user's own
  "enabledModels": [
    "xai/grok-4.6:high",
    "agentlink/wevnal/mimo-v2.6-flash-free",
    "agentlink/wevnal/kimi-k3:low",
    "agentlink/wevnal/muse-spark-1.3-contributor-free",
    "agentlink/wevnal/*-lightning-*",
    "agentlink/wevnal/nemotron-3.5-lightning-free",
    "agentlink/wevnal/space-bunny-free:high"
  ],
  "defaultProvider": "xai",
  "defaultModel": "grok-4.6"
}
`), 0o644)
			a := c.a(home)
			if err := a.Apply("model", "agentlink/wevnal/glm-5.2"); err != nil {
				t.Fatal(err)
			}
			mine := `["xai/grok-4.6:high","agentlink/wevnal/mimo-v2.6-flash-free","agentlink/wevnal/kimi-k3:low","agentlink/wevnal/muse-spark-1.3-contributor-free","agentlink/wevnal/*-lightning-*","agentlink/wevnal/nemotron-3.5-lightning-free","agentlink/wevnal/space-bunny-free:high","agentlink/wevnal/glm-5.2"]`
			if got := piList(t, settings); got != mine {
				t.Fatalf("after the pick: %s", got)
			}
			// a sync with every model still there changes nothing
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			if got := piList(t, settings); got != mine {
				t.Fatalf("sync, nothing gone: %s", got)
			}

			// the reporter's four deselected
			wevnalSave(t, "glm-5.2", "kimi-k3")
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			want := `["xai/grok-4.6:high","agentlink/wevnal/kimi-k3:low","agentlink/wevnal/*-lightning-*","agentlink/wevnal/glm-5.2"]`
			if got := piList(t, settings); got != want {
				t.Fatalf("enabledModels = %s, want %s", got, want)
			}
			if !strings.Contains(readFile(settings), "// the user's own") {
				t.Fatalf("comment lost: %s", readFile(settings))
			}
			if got := a.Field("model").Get(); got != "agentlink/wevnal/glm-5.2" {
				t.Fatalf("model = %q", got)
			}

			// one selected again comes back, at its level
			wevnalSave(t, "glm-5.2", "kimi-k3", "space-bunny-free")
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			want = `["xai/grok-4.6:high","agentlink/wevnal/kimi-k3:low","agentlink/wevnal/*-lightning-*","agentlink/wevnal/glm-5.2","agentlink/wevnal/space-bunny-free:high"]`
			if got := piList(t, settings); got != want {
				t.Fatalf("selected again: %s, want %s", got, want)
			}

			// a rename takes the remembered ones with it
			if _, err := RenameProvider("wevnal", "wv"); err != nil {
				t.Fatal(err)
			}
			if err := provider.Save(provider.Provider{ID: "wv", Name: "Wevnal", Chat: "https://wevnal.invalid/v1", Key: "k", Models: wevnalAll}); err != nil {
				t.Fatal(err)
			}
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			want = `["xai/grok-4.6:high","agentlink/wv/kimi-k3:low","agentlink/wv/*-lightning-*","agentlink/wv/glm-5.2","agentlink/wv/space-bunny-free:high","agentlink/wv/mimo-v2.6-flash-free","agentlink/wv/muse-spark-1.3-contributor-free","agentlink/wv/nemotron-3.5-lightning-free"]`
			if got := piList(t, settings); got != want {
				t.Fatalf("after the rename: %s, want %s", got, want)
			}
		})
	}
}

// A list with nothing left is removed, not written empty, and comes back
// when its model does, with the model picked meanwhile. A list the user
// removed themselves stays removed; one on other providers alone, or a
// sync that can't read what agentlink gives, leaves the file byte for byte.
func TestPiEnabledModelsEmptiedAndBack(t *testing.T) {
	home := syncHome(t)
	wevnalSave(t, wevnalAll...)
	settings := filepath.Join(home, ".pi", "agent", "settings.json")
	os.MkdirAll(filepath.Dir(settings), 0o755)
	os.WriteFile(settings, []byte(`{"enabledModels": ["agentlink/wevnal/space-bunny-free"]}`+"\n"), 0o644)
	a := pi(home)
	if err := a.Apply("model", "agentlink/wevnal/space-bunny-free"); err != nil {
		t.Fatal(err)
	}
	wevnalSave(t, "glm-5.2", "kimi-k3")
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if got := piList(t, settings); got != "(none)" {
		t.Fatalf("emptied: %s", got)
	}
	if err := a.Apply("model", "agentlink/wevnal/kimi-k3"); err != nil {
		t.Fatal(err)
	}
	wevnalSave(t, "glm-5.2", "kimi-k3", "space-bunny-free")
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if got, want := piList(t, settings), `["agentlink/wevnal/space-bunny-free","agentlink/wevnal/kimi-k3"]`; got != want {
		t.Fatalf("back: %s, want %s", got, want)
	}

	// removed by the user, then the model goes and comes back: no list made
	if err := edit.DelJSON(settings, "enabledModels"); err != nil {
		t.Fatal(err)
	}
	wevnalSave(t, "glm-5.2", "kimi-k3")
	a.Sync()
	wevnalSave(t, wevnalAll...)
	a.Sync()
	if got := piList(t, settings); got != "(none)" {
		t.Fatalf("user's removal undone: %s", got)
	}

	const theirs = `{"enabledModels": ["xai/grok-4.6", "agentlink/*/*"], "defaultProvider": "agentlink", "defaultModel": "wevnal/kimi-k3"}` + "\n"
	os.WriteFile(settings, []byte(theirs), 0o644)
	wevnalSave(t, "kimi-k3")
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(settings); got != theirs {
		t.Fatalf("other providers' and globs rewritten: %s", got)
	}

	// models.json unreadable: unknown, so nothing goes
	const one = `{"enabledModels": ["agentlink/wevnal/kimi-k3"]}` + "\n"
	os.WriteFile(settings, []byte(one), 0o644)
	os.WriteFile(filepath.Join(home, ".pi", "agent", "models.json"), []byte(`{"providers": {"agentlink": {"models": "not a list"`), 0o644)
	a.Sync()
	if got := readFile(settings); got != one {
		t.Fatalf("unreadable models.json pruned: %s", got)
	}
}

// StepCode's enabledModels (config.toml) and omp's (config.yml) are the
// same kind of list.
func TestStepCodeAndOmpEnabledModelsLoseDeselected(t *testing.T) {
	t.Run("stepcode", func(t *testing.T) {
		home, _ := museHome(t)
		wevnalSave(t, wevnalAll...)
		configPath := filepath.Join(home, ".stepcode", "config.toml")
		stepcodeWriteTestFile(t, configPath, "# mine\nenabledModels = [\"anthropic/*\", \"agentlink/wevnal/space-bunny-free:high\", \"agentlink/wevnal/kimi-k3\"]\n")
		stepcodeWriteTestFile(t, filepath.Join(home, ".stepcode", "models.json"), "{}\n")
		a := stepcode(home)
		// Connect adds the model it picks (agentlink/deepseek/pro) to the list
		if err := a.Connect(); err != nil {
			t.Fatal(err)
		}
		if err := a.Apply("model", "agentlink/wevnal/kimi-k3"); err != nil {
			t.Fatal(err)
		}
		wevnalSave(t, "kimi-k3")
		if err := a.Sync(); err != nil {
			t.Fatal(err)
		}
		cfg, err := readStepcodeTOML(configPath)
		if err != nil || cfg.Scope == nil || !slices.Equal(*cfg.Scope, []string{"anthropic/*", "agentlink/wevnal/kimi-k3", "agentlink/deepseek/pro"}) {
			t.Fatalf("enabledModels: %v %v", cfg.Scope, err)
		}
		if !strings.Contains(readFile(configPath), "# mine") {
			t.Fatalf("comment lost: %s", readFile(configPath))
		}
		wevnalSave(t, "kimi-k3", "space-bunny-free")
		if err := a.Sync(); err != nil {
			t.Fatal(err)
		}
		cfg, _ = readStepcodeTOML(configPath)
		if cfg.Scope == nil || !slices.Equal(*cfg.Scope, []string{"anthropic/*", "agentlink/wevnal/kimi-k3", "agentlink/deepseek/pro", "agentlink/wevnal/space-bunny-free:high"}) {
			t.Fatalf("back: %v", cfg.Scope)
		}
		// a disconnect still puts the user's own list back
		if err := a.Disconnect(); err != nil {
			t.Fatal(err)
		}
		cfg, _ = readStepcodeTOML(configPath)
		if cfg.Scope == nil || !slices.Equal(*cfg.Scope, []string{"anthropic/*", "agentlink/wevnal/space-bunny-free:high", "agentlink/wevnal/kimi-k3"}) {
			t.Fatalf("after disconnect: %v", cfg.Scope)
		}
	})
	t.Run("omp", func(t *testing.T) {
		home, configPath, _ := ompScopeHome(t)
		wevnalSave(t, wevnalAll...)
		os.WriteFile(configPath, []byte("# mine\nenabledModels:\n  - provider-a/model-a # first\n  - agentlink/wevnal/space-bunny-free:high\n  - agentlink/wevnal/kimi-k3\n  - path: /work\n    models: [agentlink/wevnal/mimo-v2.6-flash-free]\n"), 0o644)
		a := omp(home)
		if err := a.Field("model").Set("agentlink/wevnal/kimi-k3"); err != nil {
			t.Fatal(err)
		}
		wevnalSave(t, "kimi-k3")
		if err := a.Sync(); err != nil {
			t.Fatal(err)
		}
		if got := edit.GetYAMLList(configPath, "enabledModels"); !slices.Equal(got, []string{"provider-a/model-a", "agentlink/wevnal/kimi-k3"}) {
			t.Fatalf("enabledModels: %q", got)
		}
		_, raw := ompScopeRead(t, configPath)
		if !strings.Contains(raw, "# first") || !strings.Contains(raw, "path: /work") {
			t.Fatalf("comment or folder entry lost:\n%s", raw)
		}
		wevnalSave(t, "kimi-k3", "space-bunny-free")
		if err := a.Sync(); err != nil {
			t.Fatal(err)
		}
		if got := edit.GetYAMLList(configPath, "enabledModels"); !slices.Equal(got, []string{"provider-a/model-a", "agentlink/wevnal/kimi-k3", "agentlink/wevnal/space-bunny-free:high"}) {
			t.Fatalf("back: %q", got)
		}
	})
}

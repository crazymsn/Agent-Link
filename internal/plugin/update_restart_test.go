package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestTerminalUpdateRestartsTheHost (#952): `agentlink plugin update` in a
// terminal installs a newer version without changing plugins.json, so the
// app running beside it went on with the host it had (0.1.17's code after
// the update to 0.1.18) until that was killed. What bun installs in Dir()
// counts as a change another agentlink made; this agentlink's own restart does
// not.
func TestTerminalUpdateRestartsTheHost(t *testing.T) {
	storeSandbox(t)
	writeList(t, `{"plugins":[{"spec":"@agentlink-community/opencode-factory-auth"}]}`)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(Dir(), "package.json")
	lock := filepath.Join(Dir(), "bun.lock")
	write := func(p, body string, at time.Time) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	then := time.Now().Add(-time.Hour)
	write(pkg, `{"dependencies":{"@agentlink-community/opencode-factory-auth":"0.1.17"}}`, then)
	write(lock, `"@agentlink-community/opencode-factory-auth@0.1.17"`, then)

	prevStale := hostStale.Swap(false)
	t.Cleanup(func() {
		if prevStale {
			hostStale.Store(true)
		}
	})
	checkList() // the app has seen the plugins as they are
	hostStale.Store(false)
	checkList()
	if hostStale.Load() {
		t.Fatal("nothing changed, and the host is stale")
	}

	// the terminal's update: bun rewrites both, plugins.json is as it was
	list, _ := os.ReadFile(listPath())
	write(pkg, `{"dependencies":{"@agentlink-community/opencode-factory-auth":"0.1.18"}}`, then.Add(time.Minute))
	write(lock, `"@agentlink-community/opencode-factory-auth@0.1.18"`, then.Add(time.Minute))
	if now, _ := os.ReadFile(listPath()); string(now) != string(list) {
		t.Fatal("plugins.json changed")
	}
	checkList()
	if !hostStale.Swap(false) {
		t.Fatal("the plugins installed changed, and the running host is not stale: it goes on with the old version")
	}

	// this agentlink's own install restarts the host itself: no second restart
	write(lock, `"@agentlink-community/opencode-factory-auth@0.1.19"`, then.Add(2*time.Minute))
	Restart()
	checkList()
	if hostStale.Load() {
		t.Fatal("this agentlink's own restart was taken for another agentlink's change")
	}
}

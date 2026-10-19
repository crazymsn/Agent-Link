package agent

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crazymsn/Agent-Link/internal/edit"
	"github.com/crazymsn/Agent-Link/internal/gateway"
	"github.com/crazymsn/Agent-Link/internal/provider"
)

// corpProxy is a company's reverse proxy in front of the gateway, with a
// path of its own before agentlink's (#1985).
const corpProxy = "https://ai.corp.example/agentlink"

// localProxy is the reporter's: a company-mandated compliance proxy on this
// machine, its route before agentlink's paths.
const localProxy = "http://127.0.0.1:8788/claude-code-anthropic/cc-7f3a9e"

// fakeAgentLinkAt answers agentlinkAt from answers (a base missing is no agentlink),
// as the probe would have, and says which bases were asked.
func fakeAgentLinkAt(t *testing.T, answers map[string]bool) func() []string {
	t.Helper()
	var mu sync.Mutex
	var asked []string
	was := agentlinkAt
	agentlinkAt = func(base string) (bool, bool) {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, base)
		return answers[base], true
	}
	t.Cleanup(func() { agentlinkAt = was })
	return func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), asked...) }
}

// davisjiahao (#1985): every agent connected to agentlink, then pointed by the
// user at a proxy that forwards to the gateway, agentlink's key and model
// kept: a company's nginx (https://ai.corp.example/agentlink), or the
// reporter's own, a compliance proxy on this machine every request must go
// through (http://127.0.0.1:8788/claude-code-anthropic/<route>). That is
// still agentlink: no agent's Check calls it changed, and the sync every
// start runs keeps the proxy's address, its path included. The same
// address where something else answers (a relay CC Switch put there) is
// still told, for every agent but Codex, which keeps an address with
// agentlink's key in its provider table as it did for #816 and tries it
// (unreachable).
func TestReverseProxyIsStillAgentLink(t *testing.T) {
	// named so the test's folders, which carry the name, hold no address
	t.Run("nginx", func(t *testing.T) { reverseProxyIsStillAgentLink(t, corpProxy) })
	t.Run("local", func(t *testing.T) { reverseProxyIsStillAgentLink(t, localProxy) })
}

func reverseProxyIsStillAgentLink(t *testing.T, proxy string) {
	t.Setenv("AGENT_LINK_ADDR", "")
	host := strings.SplitN(strings.SplitN(proxy, "://", 2)[1], "/", 2)[0]
	home, _ := codexHome(t, "", "")
	managed := claudeManaged
	claudeManaged = func() string { return filepath.Join(home, "managed-settings.json") }
	t.Cleanup(func() { claudeManaged = managed })
	os.MkdirAll(filepath.Join(home, ".hanako", "agents", "hana"), 0o755)
	os.WriteFile(filepath.Join(home, ".hanako", "agents", "hana", "config.yaml"), []byte("agent:\n  name: Hana\n"), 0o644)
	t.Setenv("ASTRBOT_ROOT", "")
	writeFile(t, filepath.Join(home, ".astrbot", "data", "cmd_config.json"), astrbotConfig)
	setPort(t, 3591)
	var agents []*Agent
	for _, a := range All() {
		// Alma is told through its running app, and Cursor's local
		// gateway is started by launchctl: neither is in the sandbox
		if a.Check == nil || a.Launch != nil || a.WSL != "" || a.Native != nil || a.ID == "alma" || a.ID == "cursorlocal" {
			continue
		}
		var key, want string
		for _, g := range a.Fields {
			for _, o := range g.Options(a.Values()) {
				if want == "" && (o.Ref == "fake/m1" || o.Value == agentlinkID) {
					key, want = g.Key, o.Value
				}
			}
		}
		if want == "" {
			t.Fatalf("%s: no field takes agentlink's models", a.ID)
		}
		if err := a.Apply(key, want); err != nil {
			t.Fatalf("%s: %v", a.ID, err)
		}
		if c := a.Check(); c != "" {
			t.Fatalf("%s connected, yet: %s", a.ID, c)
		}
		agents = append(agents, a)
	}
	moved := filesWith(home, "127.0.0.1:3591")
	if len(moved) < 20 {
		t.Fatalf("only %d files have the gateway's address: %v", len(moved), moved)
	}
	for _, p := range moved {
		b, _ := os.ReadFile(p)
		os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "http://127.0.0.1:3591", proxy)), 0o644)
	}

	asked := fakeAgentLinkAt(t, map[string]bool{proxy: true})
	for _, a := range agents {
		if c := a.Check(); c != "" {
			t.Errorf("%s behind the proxy: %s", a.ID, c)
		}
		if d := a.Drift(); d != nil && d.Kind == "unwired" {
			t.Errorf("%s behind the proxy drifted: %+v", a.ID, d)
		}
	}
	for _, b := range asked() {
		if b != proxy {
			t.Errorf("asked %s, not the proxy's own address", b)
		}
	}

	// a relay at that address, not agentlink: told
	fakeAgentLinkAt(t, map[string]bool{})
	for _, a := range agents {
		if c := a.Check(); (c == "") != (a.ID == "codex") {
			t.Errorf("%s at a relay: %q", a.ID, c)
		}
	}

	// the sync keeps the proxy, its /agentlink too
	fakeAgentLinkAt(t, map[string]bool{proxy: true})
	if err := provider.Save(provider.Provider{ID: "fake2", Name: "Fake2", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}}); err != nil {
		t.Fatal(err)
	}
	SyncCatalog()
	for _, p := range moved {
		b, _ := os.ReadFile(p)
		s, name := string(b), strings.TrimPrefix(p, home)
		if strings.Contains(s, "127.0.0.1:3591") {
			t.Errorf("%s is back on this machine's gateway", name)
		}
		if !strings.Contains(s, proxy) || strings.Count(s, host) != strings.Count(s, proxy) {
			t.Errorf("%s lost the proxy's path:\n%s", name, s)
		}
	}
	for _, a := range agents {
		if c := a.Check(); c != "" {
			t.Errorf("%s after a sync: %s", a.ID, c)
		}
	}
}

// Claude Code's managed settings pointing it at the company's proxy to
// agentlink win over the user's, and still reach agentlink: not drift.
func TestClaudeManagedProxyIsStillAgentLink(t *testing.T) {
	home, _ := codexHome(t, "", "")
	managed := claudeManaged
	claudeManaged = func() string { return filepath.Join(home, "managed-settings.json") }
	t.Cleanup(func() { claudeManaged = managed })
	cl := claude(home)
	if err := cl.Apply("model", "fake/m1"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(claudeManaged(), []byte(`{"env":{"ANTHROPIC_BASE_URL":"`+corpProxy+`"}}`), 0o644)
	fakeAgentLinkAt(t, map[string]bool{corpProxy: true})
	if d := cl.Drift(); d != nil {
		t.Fatalf("managed proxy to agentlink: %+v", d)
	}
}

// Where it reaches agentlink isn't known until the address has been tried,
// in the background: until then it isn't told as changed (Reconnect would
// write the proxy away), once tried and not agentlink it is; an address on
// this machine's loopback is agentlink's own on an older port, told at once,
// never tried.
func TestReverseProxyTriedFirst(t *testing.T) {
	home, path := claudeSettings(t, `{}`)
	saveDeepseek(t)
	cl := claude(home)
	if err := cl.Apply("model", "deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	was, wasAt, age := agentlinkProbe, agentlinkAt, reachAge
	release := make(chan struct{})
	var mu sync.Mutex
	var tried []string
	agentlinkProbe = func(base string) bool {
		<-release
		mu.Lock()
		tried = append(tried, base)
		mu.Unlock()
		return false
	}
	agentlinkAt = func(base string) (bool, bool) {
		return probed("agentlink "+base, func() bool { return agentlinkProbe(base) })
	}
	reachAge = time.Hour
	reachForget()
	t.Cleanup(func() { agentlinkProbe, agentlinkAt, reachAge = was, wasAt, age; reachForget() })

	edit.SetJSON(path, edit.KV{Path: "env.ANTHROPIC_BASE_URL", Value: corpProxy + "/"})
	if d := cl.Drift(); d != nil {
		t.Fatalf("told before the proxy was tried: %+v", d)
	}
	close(release)
	if d := driftSettled(t, cl); d == nil || d.Kind != "unwired" || !strings.Contains(d.Detail, corpProxy) {
		t.Fatalf("not agentlink there, yet %+v", d)
	}
	if len(tried) != 1 || tried[0] != corpProxy {
		t.Fatalf("tried %v", tried)
	}

	edit.SetJSON(path, edit.KV{Path: "env.ANTHROPIC_BASE_URL", Value: "http://127.0.0.1:1"})
	if d := cl.Drift(); d == nil || d.Kind != "unwired" {
		t.Fatalf("an older port on loopback: %+v", d)
	}
	if len(tried) != 1 {
		t.Fatalf("loopback tried: %v", tried)
	}
}

// The probe takes agentlink's own answer at the proxy's path (the gateway's
// GET /, forwarded), and nothing else: a relay's, an error page, agentlink's
// name in a page that isn't 200.
func TestAgentLinkProbe(t *testing.T) {
	gw := gateway.New().Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/agentlink/"):
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/agentlink")
			gw.ServeHTTP(w, r)
		case r.URL.Path == "/broken/":
			w.WriteHeader(502)
			w.Write([]byte(`{"name":"agentlink"}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"message":"not found"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	for base, want := range map[string]bool{
		srv.URL + "/agentlink":  true,
		srv.URL + "/agentlink/": true,
		srv.URL + "/relay":   false,
		srv.URL + "/broken":  false,
		"http://127.0.0.1:1": false,
	} {
		if got := realAgentLinkProbe(base); got != want {
			t.Errorf("%s: %v, want %v", base, got, want)
		}
	}
}

// Codex at the proxy (https, no port, a path of its own): kept as its
// gateway as #816's address on another host is, written again there, and
// a request its log says went there reached agentlink (bypassed was told
// from the host alone); one to chatgpt.com still didn't.
func TestCodexBehindReverseProxy(t *testing.T) {
	home, _ := codexAt(t, corpProxy)
	cx := codex(home)
	cfg := filepath.Join(home, ".codex", "config.toml")
	if g := codexKeptGateway(cfg); g != corpProxy {
		t.Fatalf("kept %q", g)
	}
	if err := cx.Apply("model", "fake/m1"); err != nil {
		t.Fatal(err)
	}
	if u, _ := edit.GetTOMLTop(cfg, "openai_base_url"); u != corpProxy+gateway.CodexPath {
		t.Fatalf("written back to %s", u)
	}
	db, err := sql.Open("sqlite", filepath.Join(home, ".codex", "logs_2.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE logs (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, ts_nanos INTEGER NOT NULL,
		level TEXT NOT NULL, target TEXT NOT NULL, feedback_log_body TEXT)`); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(2 * time.Second).Unix()
	log := func(body string) {
		at++
		db.Exec(`INSERT INTO logs (ts, ts_nanos, level, target, feedback_log_body) VALUES (?, 0, 'INFO', 'codex_api::endpoint::responses_websocket', ?)`,
			at, "responses_websocket.connect{}: "+body)
	}
	log("connecting to websocket: wss://ai.corp.example/agentlink/backend-api/codex/responses")
	if d := codex(home).Drift(); d != nil {
		t.Fatalf("reached agentlink through the proxy: %+v", d)
	}
	log("connecting to websocket: wss://chatgpt.com/backend-api/codex/responses")
	if d := codex(home).Drift(); d == nil || d.Kind != "bypassed" {
		t.Fatalf("to chatgpt.com: %+v", d)
	}
	// one the gateway saw after it: not bypassed, wherever the log says
	seen := lastSeen
	lastSeen = func(string) time.Time { return time.Unix(at, 0) }
	t.Cleanup(func() { lastSeen = seen })
	if d := codex(home).Drift(); d != nil {
		t.Fatalf("the gateway saw it: %+v", d)
	}
}

// The sync keeps a path before agentlink's only where every URL at the
// address has it.
func TestAwayPath(t *testing.T) {
	g := "http://127.0.0.1:3591"
	for _, c := range []struct{ was, data, want string }{
		{`{"u":"https://x.example/agentlink"}`, `{"u":"` + g + `"}`, "/agentlink"},
		{`{"u":"https://x.example/agentlink/v1"}`, `{"u":"` + g + `/v1"}`, "/agentlink"},
		{`{"u":"https://x.example/agentlink","v":"https://x.example/agentlink/v1"}`, `{"u":"` + g + `","v":"` + g + `/v1"}`, "/agentlink"},
		{`{"u":"https://x.example","v":"https://x.example/v1"}`, `{"u":"` + g + `","v":"` + g + `/v1"}`, ""},
		{`{"u":"https://x.example/v1"}`, `{"u":"` + g + `/v1"}`, ""},
		{`{"u":"https://x.example/a/v1","v":"https://x.example/b/v1"}`, `{"u":"` + g + `/v1","v":"` + g + `/v1"}`, ""},
	} {
		if got := awayPath([]byte(c.was), []byte(c.data), g, "https://x.example"); got != c.want {
			t.Errorf("%s: %q, want %q", c.was, got, c.want)
		}
	}
}

// Codex signed in to ChatGPT reaches agentlink by openai_base_url alone,
// agentlink not its provider: at the proxy that is still agentlink, though the
// address's host isn't the one isCodexGatewayOn is asked about.
func TestCodexSignedInBehindReverseProxy(t *testing.T) {
	home, _ := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, "")
	cx := codex(home)
	if err := cx.Apply("model", "fake/m1"); err != nil {
		t.Fatal(err)
	}
	if p, _ := edit.GetTOMLTop(cx.Path, "model_provider"); p == agentlinkID {
		t.Fatal("signed in, yet agentlink is Codex's provider")
	}
	b, _ := os.ReadFile(cx.Path)
	if !strings.Contains(string(b), gateway.URL()) {
		t.Fatalf("no gateway address:\n%s", b)
	}
	os.WriteFile(cx.Path, []byte(strings.ReplaceAll(string(b), gateway.URL(), corpProxy)), 0o644)
	if u, _ := edit.GetTOMLTop(cx.Path, "openai_base_url"); u != corpProxy+gateway.CodexPath {
		t.Fatalf("openai_base_url %q", u)
	}
	if c := codex(home).Check(); c != "" {
		t.Fatalf("behind the proxy: %s", c)
	}
}

// The reporter's case (#1985, davisjiahao): Claude Code must go through a
// compliance proxy on this machine (http://127.0.0.1:8788/<route>, which
// forwards to the gateway). Once agentlink has answered there, picking a
// model in agentlink keeps the proxy (it wrote 127.0.0.1:3425 back, bypassing
// the proxy the company requires), as does Reconnect, which isn't offered,
// and the sync. A relay there is agentlink's to point away from on a pick.
func TestClaudeBehindLocalProxyKeepsIt(t *testing.T) {
	home, path := claudeSettings(t, `{}`)
	saveDeepseek(t)
	cl := claude(home)
	if err := cl.Apply("model", "deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	edit.SetJSON(path, edit.KV{Path: "env.ANTHROPIC_BASE_URL", Value: localProxy})
	fakeAgentLinkAt(t, map[string]bool{localProxy: true})
	url := func() string { u, _ := edit.GetJSON(path, "env.ANTHROPIC_BASE_URL"); return u }

	if d := cl.Drift(); d != nil {
		t.Fatalf("behind the proxy: %+v", d)
	}
	if err := cl.Apply("model", "deepseek/flash"); err != nil {
		t.Fatal(err)
	}
	if u := url(); u != localProxy {
		t.Fatalf("a pick wrote %s", u)
	}
	if m := cl.Field("model").Get(); m != "deepseek/flash" {
		t.Fatalf("model %q", m)
	}
	if d := cl.Drift(); d != nil {
		t.Fatalf("after a pick: %+v", d)
	}
	// a tier given a model of its own keeps it on the next pick, as on
	// the gateway's own address
	edit.SetJSON(path, edit.KV{Path: "env." + tierEnv("haiku"), Value: "deepseek/pro"})
	if err := cl.Apply("model", "deepseek/flash"); err != nil {
		t.Fatal(err)
	}
	if v, _ := edit.GetJSON(path, "env."+tierEnv("haiku")); v != "deepseek/pro" {
		t.Fatalf("haiku's own model went: %q", v)
	}
	if err := provider.Save(provider.Provider{ID: "fake2", Name: "Fake2", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}}); err != nil {
		t.Fatal(err)
	}
	SyncCatalog()
	if u := url(); u != localProxy {
		t.Fatalf("the sync wrote %s", u)
	}

	// a relay there, not agentlink: told, and a pick puts agentlink back
	fakeAgentLinkAt(t, map[string]bool{})
	if d := cl.Drift(); d == nil || d.Kind != "unwired" {
		t.Fatalf("a relay: %+v", d)
	}
	if err := cl.Apply("model", "deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	if u := url(); u != gateway.URL() {
		t.Fatalf("a pick at a relay left %s", u)
	}
}

package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/catalog"
	"github.com/crazymsn/Agent-Link/internal/provider"
)

func TestFixedProviderModelsActionAllowed(t *testing.T) {
	for _, action := range []string{"models", "unfetch", "off", "on"} {
		if !fixedProviderActionAllowed(action) {
			t.Fatalf("fixed provider action %q was blocked", action)
		}
	}
	for _, action := range []string{"delete", "route", "rename", "detect"} {
		if fixedProviderActionAllowed(action) {
			t.Fatalf("fixed provider action %q was allowed", action)
		}
	}
}

func TestFixedProviderSwitchActions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := provider.MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(action string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/"+action, strings.NewReader(`{"id":"`+provider.AgentLinkProviderID+`"}`)))
		return w
	}
	if w := post("off"); w.Code != http.StatusOK {
		t.Fatalf("off: %d %s", w.Code, w.Body)
	}
	p, err := provider.Find(provider.AgentLinkProviderID)
	if err != nil || !p.Off {
		t.Fatalf("fixed provider after off = %#v, %v", p, err)
	}
	if w := post("on"); w.Code != http.StatusOK {
		t.Fatalf("on: %d %s", w.Code, w.Body)
	}
	if err := provider.Save(provider.Provider{ID: provider.AgentLinkProviderID, Key: "sk-kept-while-off", Models: []string{"model-a"}, Off: true}); err != nil {
		t.Fatal("saving the fixed provider while off:", err)
	}
	p, err = provider.Find(provider.AgentLinkProviderID)
	if err != nil || !p.Off || p.Key != "sk-kept-while-off" {
		t.Fatalf("fixed provider save lost off state: %#v, %v", p, err)
	}
}

// Refresh and Test models ask with the key the editor has, before a Save
// (the user: a key pasted over the saved one only errored, and Save closes
// the editor); the saved key stays as it was.
func TestFixedProviderModelsRequestKeepsCanonicalSettings(t *testing.T) {
	p := provider.AgentLinkProvider()
	p.Key = "sk-old"
	in := provider.Provider{
		Key:       "sk-new",
		Chat:      "https://attacker.example/v1",
		Responses: "https://attacker.example/responses",
		Proxy:     "http://attacker.example:8080",
		Headers:   map[string]string{"Authorization": "attacker"},
	}
	got := typedModelsRequest(p, in, stringPtr("http://attacker.example:8080"), boolPtr(true))
	if got.Key != "sk-new" || got.Chat != provider.AgentLinkProviderURL || got.Responses != provider.AgentLinkProviderURL || got.Anthropic != "https://api.meding.site" || got.Gemini != provider.AgentLinkProviderURL || got.Proxy != "" || got.Headers != nil {
		t.Fatalf("fixed provider model request was mutable: %#v", got)
	}
}

func TestFixedProviderTestRequestKeepsCanonicalSettings(t *testing.T) {
	p := provider.AgentLinkProvider()
	p.Key = "sk-old"
	in := provider.Provider{
		Key:     "sk-new",
		Chat:    "https://attacker.example/v1",
		Proxy:   "http://attacker.example:8080",
		Headers: map[string]string{"Authorization": "attacker"},
	}
	got := typedModelsRequest(p, in, stringPtr("http://attacker.example:8080"), boolPtr(true))
	if got.Key != "sk-new" || got.Chat != provider.AgentLinkProviderURL || got.Responses != provider.AgentLinkProviderURL || got.Anthropic != "https://api.meding.site" || got.Gemini != provider.AgentLinkProviderURL || got.Proxy != "" || got.Headers != nil {
		t.Fatalf("fixed provider test request was mutable: %#v", got)
	}
}

func stringPtr(s string) *string { return &s }
func boolPtr(v bool) *bool       { return &v }

func TestFixedProviderHTTPSavePersistsEditorKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := provider.MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	body := `{"id":"` + provider.AgentLinkProviderID + `","key":"sk-gui-saved","models":["model-a"],"noModels":false}`
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/provider/save", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	p, err := provider.Find(provider.AgentLinkProviderID)
	if err != nil || p.Key != "sk-gui-saved" || p.Chat != provider.AgentLinkProviderURL {
		t.Fatalf("saved fixed provider = %#v, err %v", p, err)
	}
}

func TestFixedProviderHTTPSaveClearsEditorKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := provider.MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: provider.AgentLinkProviderID, Key: "sk-old"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/provider/save", strings.NewReader(`{"id":"`+provider.AgentLinkProviderID+`","key":"","clearKey":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("clear key save: %d %s", rec.Code, rec.Body)
	}
	p, err := provider.Find(provider.AgentLinkProviderID)
	if err != nil || p.Key != "" {
		t.Fatalf("fixed key after HTTP clear = %q, err %v", p.Key, err)
	}
}

func TestFixedProviderClearFetchedModels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := provider.MigrateFixedProvider(); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive(provider.AgentLinkProviderID, provider.AgentLinkProviderURL, []catalog.Model{{ID: "model-a"}}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/provider/unfetch", strings.NewReader(`{"id":"`+provider.AgentLinkProviderID+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("clear fetched models: %d %s", rec.Code, rec.Body)
	}
	if _, _, ok := catalog.Live(provider.AgentLinkProviderID); ok {
		t.Fatal("fixed provider fetched models survived clear")
	}
}

func TestProviderTypedKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-new" || r.Header.Get("X-Team") != "b" {
			http.Error(w, `{"error":{"message":"invalid api key"}}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
			return
		}
		w.Write([]byte(`{"id":"x","object":"chat.completion","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer vendor.Close()
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(action, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/"+action, strings.NewReader(body)))
		return w
	}
	if w := post("save", `{"id":"relay","name":"Relay","chat":"`+vendor.URL+`/v1","key":"sk-old","headers":{"X-Team":"a"},"proxy":"direct","new":true}`); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	form := `"typed":true,"key":"sk-new","chat":"` + vendor.URL + `/v1","headers":{"X-Team":"b"}`

	// the saved key is turned away, as before
	if w := post("models", `{"id":"relay"}`); w.Code == 200 {
		t.Fatalf("the saved key fetched: %s", w.Body)
	}
	w := post("models", `{"id":"relay",`+form+`}`)
	if w.Code != 200 {
		t.Fatalf("models with the typed key: %d %s", w.Code, w.Body)
	}
	var got struct{ Count int }
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.Count != 2 {
		t.Fatalf("count %d: %s", got.Count, w.Body)
	}

	var res struct {
		Results []provider.Result `json:"results"`
	}
	json.Unmarshal(post("test", `{"id":"relay","test":["m1"]}`).Body.Bytes(), &res)
	if len(res.Results) != 1 || res.Results[0].OK {
		t.Fatalf("the saved key answered: %+v", res.Results)
	}
	w = post("test", `{"id":"relay","test":["m1"],`+form+`}`)
	res.Results = nil
	json.Unmarshal(w.Body.Bytes(), &res)
	if len(res.Results) != 1 || !res.Results[0].OK {
		t.Fatalf("the typed key didn't answer: %s", w.Body)
	}

	// nothing saved
	p, err := provider.Find("relay")
	if err != nil || p.Key != "sk-old" || p.Headers["X-Team"] != "a" {
		t.Fatalf("%v key %q headers %v", err, p.Key, p.Headers)
	}
}

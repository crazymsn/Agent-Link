package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifestPlugin(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "word-guard.middleware.js"), []byte("export function onRequest() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCommunityMagpieMetadataLoadsAsAgentLink(t *testing.T) {
	dir := writeManifestPlugin(t, `{"name":"@magpie-community/middleware-word-guard","magpie":{"middleware":"./word-guard.middleware.js","options":{"action":"reject"}}}`)
	file, only := Middleware(dir)
	if filepath.Base(file) != "word-guard.middleware.js" || !only {
		t.Fatalf("Middleware = %q, %v", file, only)
	}
	if got := OptionsExample(dir)["action"]; got != "reject" {
		t.Fatalf("OptionsExample = %#v, want community options", got)
	}
}

func TestAgentLinkMetadataWinsOverCommunityMetadata(t *testing.T) {
	dir := writeManifestPlugin(t, `{"name":"mixed","magpie":{"middleware":"./community.js","options":{"source":"magpie"}},"agentlink":{"middleware":"./agentlink.js","options":{"source":"agentlink"}}}`)
	file, _ := Middleware(dir)
	if filepath.Base(file) != "agentlink.js" {
		t.Fatalf("Middleware = %q, want Agent Link metadata", file)
	}
	if got := OptionsExample(dir)["source"]; got != "agentlink" {
		t.Fatalf("OptionsExample = %#v, want Agent Link metadata", got)
	}
}

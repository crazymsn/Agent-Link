//go:build !nogui

package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/appdir"
)

// #508: a portable agentlink on Windows still made %APPDATA%\agentlink.exe,
// WebView2's profile at Wails' default. Portable, the app's options put it
// in data\webview2; installed, they leave the default (and what is there).
func TestWindowsOptionsWebViewData(t *testing.T) {
	t.Setenv("APPIMAGE", "")
	t.Cleanup(func() { appdir.UseExecutable("") })
	r, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	appdir.UseExecutable("")
	if o := windowsOptions(); o.WebviewUserDataPath != "" || !o.DisableQuitOnLastWindowClosed {
		t.Fatalf("installed: %+v, want WebView2's default folder and no quit on last close", o)
	}

	exe := filepath.Join(r, "AgentLink", "agentlink.exe")
	if err := os.MkdirAll(filepath.Join(r, "AgentLink", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	appdir.UseExecutable(exe)
	o := windowsOptions()
	if want := filepath.Join(r, "AgentLink", "data", "webview2"); o.WebviewUserDataPath != want {
		t.Fatalf("portable: WebviewUserDataPath = %q, want %q", o.WebviewUserDataPath, want)
	}
	if !o.DisableQuitOnLastWindowClosed {
		t.Fatal("portable: quits on last window closed")
	}
}

package gui

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// registerScheme makes agent-link:// and the legacy agentlink:// links open this executable.
func registerScheme() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	for _, scheme := range []string{"agent-link", "agentlink"} {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+scheme, registry.SET_VALUE)
		if err != nil { return err }
		_ = k.SetStringValue("", "URL:"+scheme)
		_ = k.SetStringValue("URL Protocol", "")
		icon, _, err := registry.CreateKey(k, `DefaultIcon`, registry.SET_VALUE)
		if err == nil { _ = icon.SetStringValue("", `"`+exe+`",0`); icon.Close() }
		cmd, _, err := registry.CreateKey(k, `shell\open\command`, registry.SET_VALUE)
		if err != nil { k.Close(); return err }
		_ = cmd.SetStringValue("", `"`+exe+`" "%1"`)
		cmd.Close(); k.Close()
	}
	return nil
}

// keepWebData: WebView2's data folder isn't named after the program.
func keepWebData() {}

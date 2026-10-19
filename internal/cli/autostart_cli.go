package cli

import (
	"fmt"
	"runtime"

	"github.com/crazymsn/Agent-Link/internal/autostart"
)

// agentlink autostart [on|off]: whether agentlink opens at login, in the tray.
func autostartCmd(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: agentlink autostart [on|off]")
	}
	if len(args) == 1 {
		switch args[0] {
		case "on", "off":
			if err := autostart.Set(args[0] == "on"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("usage: agentlink autostart [on|off]")
		}
	}
	if autostart.Enabled() {
		if runtime.GOOS == "android" {
			fmt.Println(green.Render("✓"), "agentlink starts the gateway at boot (requires Termux:Boot)")
		} else {
			fmt.Println(green.Render("✓"), "agentlink opens at login, in the tray")
		}
	} else {
		fmt.Println("agentlink doesn't open at login")
	}
	return nil
}

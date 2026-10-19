package shortcut

import "testing"

func TestTarget(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Tools\agentlink-windows-amd64.exe`:          `C:\Tools\agentlink-windows-amd64.exe`,
		`C:\Tools\agentlink.exe.old`:                    `C:\Tools\agentlink.exe`,
		`C:\Tools\agentlink.exe.old-3`:                  `C:\Tools\agentlink.exe`,
		`C:\Tools\agentlink.exe.new`:                    `C:\Tools\agentlink.exe`,
		`C:\Tools\AGENT_LINK.EXE.OLD`:                    `C:\Tools\AGENT_LINK.EXE`,
		`C:\old\agentlink.exe`:                          `C:\old\agentlink.exe`,
		`C:\Tools\agentlink.exe.older`:                  `C:\Tools\agentlink.exe.older`,
		`/usr/local/bin/agentlink`:                      `/usr/local/bin/agentlink`,
		`C:\Users\me\Downloads\agentlink (1).exe.old-2`: `C:\Users\me\Downloads\agentlink (1).exe`,
	} {
		if got := Target(in); got != want {
			t.Errorf("Target(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStale(t *testing.T) {
	exe := "/tools/agentlink.exe"
	for _, c := range []struct {
		target, dir string
		stale       bool
	}{
		{"", "", true},                            // no shortcut yet
		{"/tools/agentlink.exe", "/tools", false},    // already this one
		{"/TOOLS/AgentLink.EXE", "/Tools/", false},   // the same, in another case
		{"/downloads/agentlink.exe", "/tools", true}, // the exe was moved
		{"/tools/agentlink.exe", "/downloads", true}, // starts in another folder
		{"/tools/agentlink.exe", "", true},           // in no folder
	} {
		if got := Stale(c.target, c.dir, exe); got != c.stale {
			t.Errorf("Stale(%q, %q) = %v, want %v", c.target, c.dir, got, c.stale)
		}
	}
}

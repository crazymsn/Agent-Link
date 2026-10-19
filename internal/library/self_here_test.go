package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// thisAgentLink makes exe the agentlink this computer runs.
func thisAgentLink(t *testing.T, exe string) {
	t.Helper()
	old := selfExe
	selfExe = func() (string, error) { return exe, nil }
	t.Cleanup(func() { selfExe = old })
}

// seedLibrary writes library.json as a agentlink before #1439 left it, or
// as a sync from another computer brought it: the servers as they are.
func seedLibrary(t *testing.T, servers ...Server) {
	t.Helper()
	l := Library{MCP: []*Server{}, Applied: map[string]*Applied{}}
	for i := range servers {
		l.MCP = append(l.MCP, &servers[i])
	}
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, path(), string(b))
}

// codexCommand is the command Codex's config.toml starts server name with.
func codexCommand(t *testing.T, h, name string) string {
	t.Helper()
	var c struct {
		Servers map[string]struct {
			Command string `toml:"command"`
		} `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal([]byte(read(t, filepath.Join(h, ".codex", "config.toml"))), &c); err != nil {
		t.Fatal(err)
	}
	return c.Servers[name].Command
}

// storedCommand is the command library.json keeps for server name.
func storedCommand(t *testing.T, name string) string {
	t.Helper()
	var l Library
	if err := json.Unmarshal([]byte(read(t, path())), &l); err != nil {
		t.Fatal(err)
	}
	for _, s := range l.MCP {
		if s.Name == name {
			return s.Command
		}
	}
	t.Fatalf("library.json has no %s", name)
	return ""
}

// #1439: agentlink-image was kept with one computer's agentlink by full path. A
// sync carried C:\Users\…\Downloads\agentlink-windows-amd64.exe to a Mac, a
// Mac's app to Windows, and a agentlink moved out of Downloads left its old
// path behind: the page said it couldn't start, and the agents were given
// a program that isn't there. The library now gives this computer's agentlink,
// to the agents and to the check, and keeps it.
func TestOwnServerFromElsewhereRunsThisAgentLink(t *testing.T) {
	for _, c := range []struct{ name, stored string }{
		{"Windows' agentlink", `C:\Users\xiaoqian\Downloads\agentlink-windows-amd64.exe`},
		{"a Mac's agentlink", "/Volumes/gone-1439/AgentLink.app/Contents/MacOS/agentlink"},
		{"moved out of Downloads", ""},
		{"a sync from a agentlink that names it", "agentlink"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := sandbox(t)
			here := filepath.Join(h, "Programs", "AgentLink", "agentlink-windows-amd64.exe")
			write(t, here, "")
			thisAgentLink(t, here)
			stored := c.stored
			if stored == "" { // it was here, in Downloads, and has gone
				stored = filepath.Join(h, "Downloads", "agentlink-windows-amd64.exe")
				write(t, stored, "")
				if err := os.Remove(stored); err != nil {
					t.Fatal(err)
				}
			}
			seedLibrary(t, Server{Name: selfServerName, Transport: "stdio", Command: stored, Args: []string{"mcp", "image"}, Agents: []string{"codex"}})

			// the page and the check read it as this computer's
			v, err := load()
			if err != nil {
				t.Fatal(err)
			}
			if got := v.server(selfServerName).Command; got != here {
				t.Fatalf("read as %q, want this agentlink %q", got, here)
			}
			// the agents are given it, and library.json keeps it
			ok(t)(Sync())
			if got := codexCommand(t, h, selfServerName); got != here {
				t.Errorf("Codex starts %q, want %q", got, here)
			}
			if got := storedCommand(t, selfServerName); got != here {
				t.Errorf("library.json keeps %q, want %q", got, here)
			}
			// and once it is, a sync has nothing to write
			if res := ok(t)(Sync()); len(res.Changed) != 0 {
				t.Errorf("a second sync changed %v", res.Changed)
			}
		})
	}
}

// Only agentlink's own is pointed at this agentlink: a server of the user's that
// runs another program, one by a name like agentlink's that isn't agentlink's
// MCP server, or a agentlink build that is on this computer stays as it is,
// in the library and in the agents, gone or not.
func TestUsersOwnServerStaysAsItIs(t *testing.T) {
	h := sandbox(t)
	here := filepath.Join(h, "Programs", "agentlink")
	write(t, here, "")
	thisAgentLink(t, here)
	otherBuild := filepath.Join(h, "dev", "agentlink-dev")
	write(t, otherBuild, "")
	mine := map[string]string{
		"gone-tool":    `C:\Users\me\tools\imagetool.exe`, // mcp image, but not agentlink
		"agentlink-cli":   `C:\Users\me\tools\agentlink-helper.exe`,
		"pinned-build": otherBuild,
		"npx-server":   "npx",
	}
	seedLibrary(t,
		Server{Name: "gone-tool", Transport: "stdio", Command: mine["gone-tool"], Args: []string{"mcp", "image"}, Agents: []string{"codex"}},
		Server{Name: "agentlink-cli", Transport: "stdio", Command: mine["agentlink-cli"], Args: []string{"serve"}, Agents: []string{"codex"}},
		Server{Name: "pinned-build", Transport: "stdio", Command: mine["pinned-build"], Args: []string{"mcp", "image"}, Agents: []string{"codex"}},
		Server{Name: "npx-server", Transport: "stdio", Command: "npx", Args: []string{"-y", "@mcp/fs"}, Agents: []string{"codex"}},
	)
	ok(t)(Sync())
	for name, want := range mine {
		if got := storedCommand(t, name); got != want {
			t.Errorf("%s: library.json has %q, want the user's %q", name, got, want)
		}
		if got := codexCommand(t, h, name); got != want {
			t.Errorf("%s: Codex starts %q, want the user's %q", name, got, want)
		}
	}
}

// A library carried to another computer names agentlink's own server's
// program as plain "agentlink", so two computers with the same library carry
// the same bundle, and a sync doesn't write one's path over the other's
// each time. Put there, it runs that computer's agentlink.
func TestCarriedOwnServerIsEachComputersAgentLink(t *testing.T) {
	h := sandbox(t)
	mac := filepath.Join(h, "mac", "AgentLink.app", "Contents", "MacOS", "agentlink")
	win := filepath.Join(h, "win", "agentlink-windows-amd64.exe")
	write(t, mac, "")
	write(t, win, "")
	own := func(cmd string) Server {
		return Server{Name: selfServerName, Transport: "stdio", Command: cmd, Args: []string{"mcp", "image"}, Agents: []string{"codex"}}
	}
	user := Server{Name: "fs", Transport: "stdio", Command: "npx", Args: []string{"-y", "@mcp/fs"}, Agents: []string{"codex"}}

	thisAgentLink(t, mac)
	seedLibrary(t, own(mac), user)
	onMac, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	thisAgentLink(t, win)
	seedLibrary(t, own(win), user)
	onWin, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	if a, b := mustJSON(t, onMac), mustJSON(t, onWin); a != b {
		t.Fatalf("one library carried two ways:\n%s\n%s", a, b)
	}
	for _, s := range onMac.MCP {
		if s.Name == selfServerName && s.Command != agentlinkCommand {
			t.Errorf("carried as %q, want %q", s.Command, agentlinkCommand)
		}
		if s.Name == "fs" && s.Command != "npx" {
			t.Errorf("the user's server carried as %q", s.Command)
		}
	}

	// the Mac's bundle put on Windows runs Windows' agentlink
	ok(t)(Put(onMac))
	if got := storedCommand(t, selfServerName); got != win {
		t.Errorf("library.json keeps %q, want %q", got, win)
	}
	if got := codexCommand(t, h, selfServerName); got != win {
		t.Errorf("Codex starts %q, want %q", got, win)
	}
	if again, _ := Collect(); mustJSON(t, again) != mustJSON(t, onMac) {
		t.Error("put and carried again, the bundle changed: the next sync would send it back")
	}

	// a bundle from a agentlink before #1439, with the Mac's path in it
	old := *onMac
	old.MCP = []*Server{{Name: selfServerName, Transport: "stdio", Command: "/Volumes/gone-1439/AgentLink.app/Contents/MacOS/agentlink", Args: []string{"mcp", "image"}, Agents: []string{"codex"}}}
	ok(t)(Put(&old))
	if got := codexCommand(t, h, selfServerName); got != win {
		t.Errorf("an old bundle: Codex starts %q, want %q", got, win)
	}
}

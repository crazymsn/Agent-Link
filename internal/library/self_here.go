package library

// agentlink's own MCP servers on whichever computer reads the library (#1439).
// InstallServer gives agentlink-image this agentlink's binary by full path, and
// that path is right only here: a library carried to another computer (a
// WebDAV or S3 sync, a backup put back) came with C:\Users\…\Downloads\
// agentlink-windows-amd64.exe on a Mac, or a Mac's app on Windows, and a
// agentlink moved out of Downloads left its old path behind. Agents were
// given a program that isn't there, and the page said it can't start.
//
// So a server that is agentlink's own — `mcp …` run by a program named
// agentlink — and whose program isn't on this computer is pointed at this
// agentlink as the library is read (heal), and a carried library names
// agentlink's own as plain "agentlink" (Collect), which each computer reads as
// its own. Two computers then carry the same library, and a sync doesn't
// write one's path over the other's each time. A agentlink program that is
// there (another build kept beside this one) is the user's pick and stays;
// a server run by any other program is never touched.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/crazymsn/Agent-Link/internal/update"
)

// selfExe is this agentlink by a path that stays put; a var for tests.
var selfExe = update.Program

// ownServer is whether s is agentlink's own: `mcp …` run by a agentlink binary,
// this computer's or another's.
func ownServer(s *Server) bool {
	return !s.Remote() && len(s.Args) > 0 && s.Args[0] == "mcp" && agentlinkBinary(s.Command)
}

// agentlinkBinary is whether cmd names a agentlink program, by a path of any
// system's: agentlink, agentlink.exe, agentlink-windows-amd64.exe, the AppImage.
func agentlinkBinary(cmd string) bool {
	base := strings.ToLower(cmd[strings.LastIndexAny(cmd, `/\`)+1:])
	base = strings.TrimSuffix(base, ".exe")
	return base == agentlinkCommand || strings.HasPrefix(base, agentlinkCommand+"-")
}

// runsHere is whether cmd is a program by full path on this computer.
func runsHere(cmd string) bool {
	if !filepath.IsAbs(cmd) {
		return false
	}
	fi, err := os.Stat(cmd)
	return err == nil && !fi.IsDir()
}

// heal points agentlink's own servers whose program isn't here at this agentlink.
func (l *Library) heal() {
	exe := ""
	for _, s := range l.MCP {
		if s == nil || !ownServer(s) || runsHere(s.Command) {
			continue
		}
		if exe == "" {
			e, err := selfExe()
			if err != nil || e == "" {
				return
			}
			exe = e
		}
		s.Command = exe
	}
}

// carried is s as another computer is given it: agentlink's own by name.
func carried(s *Server) *Server {
	c := *s
	if ownServer(s) {
		c.Command = agentlinkCommand
	}
	return &c
}

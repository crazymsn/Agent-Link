// Package appdir decides where agentlink keeps its own files: the one place
// every other package asks (#508).
//
// Installed, that is ~/.config/agentlink ($XDG_CONFIG_HOME/agentlink) for what
// agentlink keeps and ~/.cache/agentlink ($XDG_CACHE_HOME/agentlink) for what it
// can fetch again, as it always was. Portable, when a folder named "data"
// (or a file named ".portable") sits beside agentlink, everything agentlink
// keeps goes into that data folder, its caches into data/cache and the
// Windows webview's profile into data/webview2 (no Start-menu shortcut or
// App Paths entry is made either), and
// nothing of agentlink's own is written to the user's profile, the way VS
// Code's portable mode works. "Beside agentlink" is the folder holding the
// executable, with links followed; for a Mac app it is the folder holding
// AgentLink.app (a file put inside the bundle would break its signature), and
// for an AppImage the folder holding the .AppImage file, not the folder it
// is mounted at while it runs.
//
// What isn't agentlink's own stays where its owner reads it: the agents'
// configs (~/.claude, ~/.codex…), the sign-ins they keep in the system
// keychain, the folders the OS reads for autostart and link handling, and
// temporary folders.
//
// A data folder in a folder every app or download shares (/Applications on
// a Mac; the home folder, Downloads, Desktop and Documents everywhere) is
// taken for agentlink's own only when it already holds agentlink's files and the
// installed folder doesn't, or with a ".portable" marker: a "data" folder
// there is as likely another app's or the user's own. Taking one showed a
// user who had never asked for portable an empty agentlink (#989), and wrote
// agentlink's files into a Downloads\data folder a Windows user's archiver
// empties, so a provider just added was gone (tangle778 on X).
//
// The decision is made once, from the executable as it was when agentlink
// started: an update moving the running copy aside (into .agentlink-update on
// a Mac, to agentlink.exe.old on Windows) leaves it running from somewhere
// else, and its files must not move with it.
package appdir

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	// DataName is the folder beside agentlink that makes it portable and
	// holds its files.
	DataName = "data"
	// MarkerName is a file beside agentlink that makes it portable too, its
	// files then going into a data folder made beside it.
	MarkerName = ".portable"
)

var (
	once     sync.Once
	portable string // the data folder, or "" when installed
)

// Portable is the data folder agentlink keeps everything in, or "" when it
// isn't portable.
func Portable() string {
	once.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		portable = Resolve(exe)
		if portable == "" {
			if data := Passed(exe); data != "" {
				log.Printf("appdir: %s is beside agentlink but not taken for its portable data: it is in a shared folder and %s has agentlink's files too; a .portable file beside agentlink makes it portable", data, installed())
			}
		}
	})
	return portable
}

// UseExecutable decides again as if agentlink ran from exe ("" for an
// installed agentlink, whatever runs): for tests.
func UseExecutable(exe string) {
	once.Do(func() {})
	portable = ""
	if exe != "" {
		portable = Resolve(exe)
	}
}

// Resolve is the data folder of a agentlink run from exe, or "" when it isn't
// portable.
func Resolve(exe string) string {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	base := Beside(exe)
	data := filepath.Join(base, DataName)
	if fi, err := os.Stat(filepath.Join(base, MarkerName)); err == nil && !fi.IsDir() {
		return absolute(data)
	}
	if fi, err := os.Stat(data); err == nil && fi.IsDir() && !shared(base, data) {
		return absolute(data)
	}
	return ""
}

// Passed is the data folder beside a agentlink run from exe that holds
// agentlink's files but that Resolve doesn't take for its own, because it is
// in a shared folder while the installed folder has agentlink's files too, or
// "". A data folder holding none of them is someone else's and isn't named.
func Passed(exe string) string {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	base := Beside(exe)
	data := filepath.Join(base, DataName)
	if fi, err := os.Stat(data); err == nil && fi.IsDir() && hasFiles(data) && Resolve(exe) == "" {
		return absolute(data)
	}
	return ""
}

// shared says whether the data folder in dir can't be taken for agentlink's:
// dir is a folder every app or download shares, where a folder named data
// is as likely another app's or the user's own, and either data holds none
// of agentlink's files or the installed folder holds them too. One who keeps
// agentlink portable there has its files in data and never ran it installed,
// and keeps it portable; a ".portable" marker beside it makes it portable
// whatever is there.
func shared(dir, data string) bool {
	if !sharedFolder(dir) {
		return false
	}
	if !hasFiles(data) {
		return true
	}
	in := installed()
	return in != "" && hasFiles(in)
}

// hasFiles says whether dir holds agentlink's own files.
func hasFiles(dir string) bool {
	for _, f := range []string{"providers.json", "logins.json", "settings.json"} {
		if fi, err := os.Stat(filepath.Join(dir, f)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

// sharedFolder says whether dir is a folder every app or download shares:
// /Applications and ~/Applications on a Mac, and the home folder, its
// Downloads, Desktop and Documents everywhere. Tests change it.
var sharedFolder = func(dir string) bool {
	var dirs []string
	if runtime.GOOS == "darwin" {
		dirs = append(dirs, "/Applications")
	}
	if h, err := Home(); err == nil {
		dirs = append(dirs, h)
		for _, d := range []string{"Downloads", "Desktop", "Documents"} {
			dirs = append(dirs, filepath.Join(h, d))
		}
		if runtime.GOOS == "darwin" {
			dirs = append(dirs, filepath.Join(h, "Applications"))
		}
	}
	for _, d := range dirs {
		if strings.EqualFold(filepath.Clean(dir), filepath.Clean(d)) {
			return true
		}
	}
	return false
}

const (
	newAppName = "agent-link"
	legacyAppName = "mag" + "pie"
)

func appConfigRoot(base string) string {
	current := filepath.Join(base, newAppName)
	legacy := filepath.Join(base, legacyAppName)
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}

// installed is the folder an installed Agent Link keeps its files in, or ""
// with no home folder.
func installed() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); rooted(x) {
		return appConfigRoot(x)
	}
	h, err := Home()
	if err != nil {
		return ""
	}
	return appConfigRoot(filepath.Join(h, ".config"))
}

// Beside is the folder a portable agentlink run from exe keeps its data folder
// in: the executable's own, the one holding the .app on a Mac, the one
// holding the .AppImage on Linux. APPIMAGE counts only for the AppImage
// agentlink runs from (exe under its mount, APPDIR): every program an
// AppImage starts inherits both, a agentlink started from an AppImage
// terminal too.
func Beside(exe string) string {
	if img := os.Getenv("APPIMAGE"); img != "" && runtime.GOOS == "linux" && within(exe, os.Getenv("APPDIR")) {
		return filepath.Dir(img)
	}
	dir := filepath.Dir(exe)
	// …/AgentLink.app/Contents/MacOS/agentlink
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		if app := filepath.Dir(filepath.Dir(dir)); strings.EqualFold(filepath.Ext(app), ".app") {
			return filepath.Dir(app)
		}
	}
	return dir
}

// within says whether path lies in the absolute folder dir.
func within(path, dir string) bool {
	if !filepath.IsAbs(dir) {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func absolute(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// Config is the folder Agent Link keeps its settings, providers, usage and the
// rest of its own state in.
func Config() string {
	if p := Portable(); p != "" {
		return p
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); rooted(x) {
		return appConfigRoot(x)
	}
	return appConfigRoot(filepath.Join(mustHome(), ".config"))
}

// Redirected is the folder agentlink keeps its files in when it is opened
// from the Dock, the Start menu or a desktop launcher (~/.config/agentlink),
// while this agentlink reads them from another that XDG_CONFIG_HOME, set in
// the shell it was run from, names; "" when they are the same, or agentlink
// is portable. A agentlink run in such a shell sees none of what the app's
// window shows.
func Redirected() string {
	if Portable() != "" || os.Getenv("XDG_CONFIG_HOME") == "" {
		return ""
	}
	h, err := Home()
	if err != nil {
		return ""
	}
	if def := appConfigRoot(filepath.Join(h, ".config")); filepath.Clean(def) != filepath.Clean(Config()) {
		return def
	}
	return ""
}

// Cache is the folder for what agentlink can fetch again (the models.dev
// catalog, exchange rates, market lists).
func Cache() string {
	if p := Portable(); p != "" {
		return filepath.Join(p, "cache")
	}
	if x := os.Getenv("XDG_CACHE_HOME"); rooted(x) {
		return appConfigRoot(x)
	}
	return appConfigRoot(filepath.Join(mustHome(), ".cache"))
}

// WebView is the folder the Windows webview (WebView2) keeps its profile
// in — its cache, cookies and the pages' storage — or "" for its own
// default. Installed, that default is %APPDATA%\<exe name>, as it always
// was, so nothing kept there is lost; portable, it is data\webview2, so a
// portable agentlink leaves no agentlink.exe folder in the user's AppData (#508).
func WebView() string {
	if p := Portable(); p != "" {
		return filepath.Join(p, "webview2")
	}
	return ""
}

// SystemCache is the cache folder the OS names for agentlink
// (~/Library/Caches/agentlink on a Mac, %LocalAppData%\agentlink on Windows),
// which a few caches have always used; portable, it is Cache.
func SystemCache() (string, error) {
	if p := Portable(); p != "" {
		return filepath.Join(p, "cache"), nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, newAppName), nil
}

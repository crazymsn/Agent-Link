package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Agent is the agent a plugin at target adds Agent Link's wiring for: the
// file its package.json's agentlink.agent names, or the file itself for a
// path to a *.agent.js (or .mjs). only is whether it is nothing else: no
// OpenCode plugin for the host to load (no main, no exports).
func Agent(target string) (file string, only bool) {
	st, err := os.Stat(target)
	if err != nil {
		return "", false
	}
	if !st.IsDir() {
		base := strings.ToLower(filepath.Base(target))
		if strings.HasSuffix(base, ".agent.js") || strings.HasSuffix(base, ".agent.mjs") {
			return target, true
		}
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(target, "package.json"))
	if err != nil {
		return "", false
	}
	b = withPluginMetadata(b)
	var pkg struct {
		Main    string          `json:"main"`
		Exports json.RawMessage `json:"exports"`
		AgentLink struct {
			Agent string `json:"agent"`
		} `json:"agentLink"`
		Legacy struct {
			Agent string `json:"agent"`
		} `json:"agentlink"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return "", false
	}
	name := strings.TrimSpace(pkg.AgentLink.Agent)
	if name == "" {
		name = strings.TrimSpace(pkg.Legacy.Agent)
	}
	if name == "" {
		return "", false
	}
	file = filepath.Join(target, filepath.FromSlash(name))
	return file, pkg.Main == "" && len(pkg.Exports) == 0
}

// InAgentLinkOnly is whether everything a plugin at target has runs in
// agentlink's own process, its gateway middleware, its agent and what it adds
// to the window, so the host has nothing of it to load.
func InAgentLinkOnly(target string) bool {
	if Gateway(target) != "" {
		return false
	}
	mw, mwOnly := Middleware(target)
	ag, agOnly := Agent(target)
	pg, pgOnly := Window(target)
	if mw == "" && ag == "" && pg == nil {
		return false
	}
	return (mw == "" || mwOnly) && (ag == "" || agOnly) && (pg == nil || pgOnly)
}

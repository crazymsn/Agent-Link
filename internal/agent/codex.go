package agent

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/crazymsn/Agent-Link/internal/access"
	"github.com/crazymsn/Agent-Link/internal/catalog"
	"github.com/crazymsn/Agent-Link/internal/codexcat"
	"github.com/crazymsn/Agent-Link/internal/edit"
	"github.com/crazymsn/Agent-Link/internal/gateway"
	"github.com/crazymsn/Agent-Link/internal/lastgood"
	"github.com/crazymsn/Agent-Link/internal/provider"
	"github.com/crazymsn/Agent-Link/internal/sessions"
	"github.com/crazymsn/Agent-Link/internal/settings"
)

// Codex talks the OpenAI Responses API. Signed in to ChatGPT, it is
// routed through agentlink with `openai_base_url` alone: Codex keeps its
// built-in OpenAI provider and sign-in, so its own models, its threads
// (listed per provider) and the Codex app's model picker stay as they are,
// and agentlink's gateway passes its own models through to OpenAI while it
// answers the rest — the model list included, so agentlink's models join
// OpenAI's in /model. Not signed in, the built-in provider can't run, and
// agentlink is a provider of its own: a [model_providers.agentlink] table,
// `model_provider = "agentlink"`, and a model catalog file for /model. So it
// is too for Codex signed in with an API key, often one a relay issued:
// Codex then never asks for the model list (its models manager skips the
// fetch for an API key), so its picker was the models it was built with
// and the one set, agentlink's others missing, and the built-in ones went to
// OpenAI with that key (#322). So it is too for a ChatGPT account the
// Codex app holds, out of its allowance with no credits to go on with
// (provider.CodexUsedUp), which it won't send anything for, whoever serves
// the model, and when the user asks for it (the sign-in field's api): the
// Codex app is then in its API state rather than signed in to ChatGPT, its
// durable threads and remote control gone with it. A window at 100% alone
// is not that: credits carry the account on, the app keeps sending, and
// Codex stays signed in beside agentlink's models. The base URL is set then
// too: a thread started on the built-in provider is opened on it again, and
// a agentlink model picked in it would otherwise go to the ChatGPT backend.
//
// The [model_providers.agentlink] table stays once written. A thread keeps the
// provider it was started on, and one started on agentlink can't be opened
// again without the table ("Model provider `agentlink` not found"), whichever
// way Codex is routed now. Codex looks model_provider up in the tables as
// it loads config.toml (codex-rs config: "Model provider `{id}` not
// found"), so `model_provider = "agentlink"` without the table keeps it from
// loading its config at all, a new thread too: agentlink never writes the one
// without the other, takes model_provider out whenever it steps out as the
// provider (disconnected, unwired, joined beside the sign-in), and Sync
// writes the table back when something else took it.

// codexStandIn is the model Codex is set to use, when agentlink is its
// provider, for a model it names that agentlink doesn't serve. Codex asks for
// some by a name of OpenAI's whatever its provider: auto-review of an
// approval goes out as "codex-auto-review" (the guardian's reviewer model,
// "independent of current login"), which agentlink answered 404 "agentlink knows
// no model", and Codex reported the review as failed. Codex itself reviews
// with the turn's model when its catalog lacks the reviewer; so does agentlink.
// Signed in, Codex's own names go on to OpenAI (codexUpstream) instead.
func codexStandIn(path string) string {
	if p, _ := edit.GetTOMLTop(path, "model_provider"); p != agentlinkID {
		return ""
	}
	m, _ := edit.GetTOMLTop(path, "model")
	return m
}

func codex(home string) *Agent { return codexIn(here(home)) }

// The two notices agentlink's multi-agent V1 setting (#141) can carry: Codex's
// own features.multi_agent_v2 is on in the config agentlink routes and takes the
// V1 over, or agentlink couldn't read that config to tell. agentlink writes nothing
// here — the setting is the user's own to turn off.
var (
	codexV1ConflictNotice  = newNotice("Codex's own multi_agent_v2 is on in the config agentlink routes, which takes over the multi-agent V1 agentlink's models ask for; turn it off in Codex's config to run subagents on agentlink's models.")
	codexV1ReadErrorNotice = newNotice("Codex's multi-agent V1 is on, but agentlink couldn't read Codex's config to check its own multi_agent_v2.")
)

// multiAgentV2OnIn reads one [features] table for Codex's own multi_agent_v2,
// read-only: the bare multi_agent_v2 boolean, or enabled in the
// [features.multi_agent_v2] table, spelled as a table, an inline object or a
// dotted key. set reports whether agentlink recognizes the key as on or off. A
// table whose enabled is left out, or any other value (a string "true"), is
// not recognized, so agentlink reads it as nothing and warns on nothing — Codex
// itself rejects such a config at load (multi_agent_v2 is a bool or a table
// there), so this is agentlink's read of an invalid file, not Codex's default.
func multiAgentV2OnIn(features map[string]any) (on, set bool) {
	switch v := features["multi_agent_v2"].(type) {
	case bool:
		return v, true
	case map[string]any:
		enabled, ok := v["enabled"].(bool)
		return enabled, ok
	}
	return false, false
}

// codexMultiAgentV2On reports whether a Codex config explicitly turns its own
// features.multi_agent_v2 on: the bare multi_agent_v2 = true key, or enabled =
// true in the [features.multi_agent_v2] table (a table, an inline object or a
// dotted key), read at the top level and, on the legacy in-file profile shape,
// in the profile the config selects. Codex 0.133.0 applied that overlay itself
// (core/config/mod.rs: Features::from_sources with base = [features] and
// profile = [profiles.<name>.features], the profile applied on top), and
// agentlink's own Check already reads profiles.<profile> the same way; that legacy
// shape is what this mirrors. From 0.134 Codex dropped in-file profiles: an
// in-file `profile` key is an error and `--profile` selects a separate
// {profile}.config.toml layer, so this is not a claim about which profile a
// modern Codex is running under — only about the same in-file shape agentlink's
// Check reads. The whole file is decoded (edit.Read + toml.Unmarshal, as
// codexEnableEfforts does). A config agentlink can't read is an error, never taken
// for off.
func codexMultiAgentV2On(path string) (bool, error) {
	raw, err := edit.Read(path)
	if err != nil {
		return false, err
	}
	if raw == nil { // no config file: no key, nothing Codex turns V2 on with
		return false, nil
	}
	var doc map[string]any
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return false, err
	}
	top, _ := doc["features"].(map[string]any)
	on, _ := multiAgentV2OnIn(top)
	// the legacy in-file profile's features override the top level's, as Codex
	// 0.133.0 applied them (base then profile) and as agentlink's Check reads
	// profiles.<profile>: take the profile's value when it sets one, else keep
	// the top level's. Only the profile the config names counts.
	if profile, _ := doc["profile"].(string); profile != "" {
		profiles, _ := doc["profiles"].(map[string]any)
		p, _ := profiles[profile].(map[string]any)
		pf, _ := p["features"].(map[string]any)
		if pon, set := multiAgentV2OnIn(pf); set {
			on = pon
		}
	}
	return on, nil
}

// codexSubAgentsV1Warning is the notice for the config Codex is routed
// through (path, routed): settings.CodexAgentsV1 is on and Codex's own
// multi_agent_v2 is explicitly on there, so Codex's V2 wins over the V1
// agentlink's own models ask for and a subagent on one can't read its task.
// "" when the setting is off, Codex isn't routed through agentlink here, or its
// own multi_agent_v2 is off; a config agentlink can't read is told, not taken
// for off.
func codexSubAgentsV1Warning(path string, routed bool) string {
	if !codexcat.V1() || !routed {
		return ""
	}
	on, err := codexMultiAgentV2On(path)
	if err != nil {
		return codexV1ReadErrorNotice.String()
	}
	if !on {
		return ""
	}
	return codexV1ConflictNotice.String()
}

// CodexSubAgentsV1Warning is codexSubAgentsV1Warning for the Codex config
// here, as the Settings row asks it (the page can't reach the config itself).
// "" when there is no Codex, or no conflict.
func CodexSubAgentsV1Warning() string {
	a, err := Find("codex")
	if err != nil || a.Routed == nil {
		return ""
	}
	return codexSubAgentsV1Warning(a.Path, a.Routed())
}

// codexIn is Codex as it lives at a place: this machine's home, or a WSL
// distro's (see wsl.go).
// codexRunning is whether a Codex runs here, whose list a change reaches
// only once it restarts; a var so tests can say.
var codexRunning = func() bool { return Running(`(^|/)codex( |$)`) }

// codexEfforts are the effort control's choices for a model among ms:
// Default first, with the level Codex takes for it when none is set, as
// the catalog entry it reads says it, then the model's levels.
func codexEfforts(ms []catalog.Model, model string) []Option {
	e := catalog.Efforts(ms, model)
	if len(e) == 0 {
		return static("low", "medium", "high", "xhigh")
	}
	d := Option{Value: "", Note: "Codex's default for the model", Takes: codexcat.TakesEffort(ms, model)}
	if d.Takes != "" {
		d.Note += ": " + d.Takes
	}
	return append([]Option{d}, static(e...)...)
}

// catalogParses is whether b is a model list agentlink writes for Codex, the
// shape Keep reads of the file there.
func catalogParses(b []byte) bool {
	var v struct {
		Models []map[string]any `json:"models"`
	}
	return json.Unmarshal(b, &v) == nil
}

// writeOwnList writes the model list agentlink keeps for Codex at path. A file
// there that doesn't parse is copied aside first: Keep (#1450) holds
// nothing of a file it can't read, so writing the new list over one would
// end the keys the user added by hand with no copy (#2263's rule for
// agentlink's state files, which this file had missed). An error stops the
// write.
func writeOwnList(path string, b []byte) error {
	if err := lastgood.KeepUnparsed(path, catalogParses); err != nil {
		return err
	}
	return edit.WriteAtomic(path, b)
}

func codexIn(at place) *Agent {
	dir := at.codexHome()
	path := filepath.Join(dir, "config.toml")
	catalogPath := filepath.Join(dir, "agent-link-models.json")
	get := func(k string) string { v, _ := edit.GetTOMLTop(path, k); return v }
	// this machine's Codex reaches the gateway at the address its config
	// gives, when that is agentlink's on another host: the Codex app on
	// Windows with its agent in WSL reads Windows' config.toml from WSL,
	// where 127.0.0.1 is the distro's own, so the user points it at
	// Windows as WSL sees it, or a forward of theirs (#816); agentlink keeps
	// that address, checks against it and writes it again
	if at.spell == nil && at.base == nil {
		at.base = func() string { return codexKeptGateway(path) }
	}
	// the catalog as the config names it: relative to config.toml on
	// Windows, where a C:\ path is no file to the same Codex reading it
	// from WSL (#816); Codex reads a relative one from its home
	catalogRef := at.native(catalogPath)
	if at.spell == nil && runtime.GOOS == "windows" {
		catalogRef = filepath.Base(catalogPath)
	}
	// ownCatalog: the config names agentlink's catalog, by any spelling
	ownCatalog := func(c string) bool {
		if c == "" {
			return false
		}
		if c == catalogRef || c == at.native(catalogPath) {
			return true
		}
		return at.spell == nil && !filepath.IsAbs(c) && filepath.Clean(filepath.Join(dir, c)) == filepath.Clean(catalogPath)
	}
	// ownList is agentlink's model list for Codex as written to catalogPath:
	// what the user added by hand to the list there (a key agentlink doesn't
	// write, supports_reasoning_summaries on a model agentlink gave none) is
	// kept, not written over by the next model switch or sync (#1450).
	// Each model's default effort is the one this config starts new
	// sessions at, or the model's nearest (codexcat.ModelDefault), which
	// Codex takes on a switch to it.
	list := func() []byte { return codexcat.CatalogAt(agentlinkModels("codex"), get("model_reasoning_effort")) }
	ownList := func() []byte {
		cur, _ := edit.Read(catalogPath)
		return codexcat.Keep(cur, list())
	}
	// dropList takes the list away as agentlink steps out as Codex's
	// provider; one holding what the user added stays, which the config
	// no longer names, for ownList to keep when agentlink is back
	dropList := func() {
		cur, err := edit.Read(catalogPath)
		if err != nil {
			return
		}
		b := list()
		if string(codexcat.Keep(cur, b)) == string(b) {
			// one that doesn't parse is copied aside before it goes: Keep
			// held nothing of it, so removing it outright would end the
			// user's own keys with no copy
			if err := lastgood.KeepUnparsed(catalogPath, catalogParses); err != nil {
				return
			}
			os.Remove(catalogPath)
		}
	}
	asProvider := func() bool { return get("model_provider") == agentlinkID }
	viaBase := func() bool {
		u := get("openai_base_url")
		return isCodexGatewayOn(u, at.host()) || strings.TrimSuffix(u, "/") == at.codexURL()
	}
	routed := func() bool { return asProvider() || viaBase() }
	// joined: connected by Join, on a model of Codex's own (its last pick)
	// with agentlink's beside it, by the base URL
	joined := func() bool { return viaBase() && stashLoad()[at.key("codex.joined")] == "1" }
	// beside: set on one of agentlink's models beside the ChatGPT sign-in, by
	// the base URL, Codex's own models in its list too: one of them written
	// in by Codex (its /model, or a Codex app still running on its old
	// pick) leaves it connected, its config still on agentlink's gateway
	// (#940: it read as not connected), the change told as drift. One
	// picked on the Agents page still takes agentlink out (set).
	beside := func() bool { return viaBase() && stashLoad()[at.key("codex.beside")] == "1" }
	// chatGPT is whether Codex is signed in to ChatGPT. With account
	// discovery off (#1535) auth.json stays unread, and Codex is taken to be
	// as agentlink last wired it, beside a ChatGPT sign-in or not, so turning
	// discovery off never rewires Codex by itself.
	//
	// A Codex that reaches the gateway from another machine (a WSL distro
	// under NAT, or AGENT_LINK_ADDR pinned beyond loopback) is wired as one
	// with no ChatGPT sign-in: beside its sign-in it would send the gateway
	// its ChatGPT token, and from there the gateway takes a named key
	// alone, so every request was a 401 (#2145, lazydao). With agentlink as
	// its provider it sends the key agentlink gives it (gwKey); its own models
	// still go straight to OpenAI on its sign-in.
	chatGPT := func() bool {
		if onAnotherMachine(at.gw()) {
			return false
		}
		if settings.AccountDiscoveryOff() {
			return joined() || beside()
		}
		return codexChatGPT(dir)
	}
	// agentlink is in Codex's config: on one of agentlink's models, or joined;
	// or, routed, on the model of the group agentlink set it to as Codex
	// spells it (gpt-6.1-sol for group/auto-gpt-6-1-sol, #750), which the
	// gateway takes as that group
	wired := func() bool {
		m := get("model")
		return isAgentLink(m) || joined() || beside() || routed() && setAsGroup(cmp.Or(at.id, "codex"), "model", m)
	}
	models := func() []catalog.Model {
		switch {
		case asProvider():
			return agentlinkModels("codex")
		case viaBase():
			return append(catalog.Codex(), agentlinkModels("codex")...)
		}
		return catalog.Codex()
	}
	// relist hands Codex its models again after the effort new sessions
	// start at changed, which their default efforts follow: agentlink's
	// catalog written again, or the list the gateway hands Codex beside
	// its sign-in marked stale (its tag carries the effort)
	relist := func() error {
		switch {
		case asProvider() && ownCatalog(get("model_catalog_json")):
			b := ownList()
			if cur, _ := edit.Read(catalogPath); string(cur) != string(b) {
				return writeOwnList(catalogPath, b)
			}
		case viaBase():
			return codexStaleCache(filepath.Join(dir, "models_cache.json"), provider.CodexListTag())
		}
		return nil
	}
	var dropSubEffort func() error
	// keep the effort valid for the model: one set that the model doesn't
	// take becomes the model's nearest (xhigh on a group of high and max
	// becomes max, codexcat.ModelDefault), not the middle of the road. One
	// left unset stays unset — Codex then takes the model's own default,
	// which is what Default on the effort control asked for (lgtm: Default
	// turned into medium on its own). Routed, the Codex app offers
	// agentlink's models' efforts too (#310).
	settle := func() error {
		if routed() {
			if err := codexEnableEfforts(path, agentlinkModels("codex")); err != nil {
				return err
			}
		}
		ms := models()
		model, effort := get("model"), get("model_reasoning_effort")
		if e := catalog.Efforts(ms, model); len(e) > 0 && effort != "" && !contains(e, effort) {
			if err := edit.SetTOMLTop(path, edit.KV{Path: "model_reasoning_effort", Value: codexcat.ModelDefault(e, effort)}); err != nil {
				return err
			}
			if err := relist(); err != nil {
				return err
			}
		}
		return dropSubEffort()
	}
	// agentlink as a provider Codex can name; its threads started on agentlink do
	// Codex's background compaction, as Settings has it (#2490): written
	// where Codex is routed through agentlink, taken back where it isn't
	asyncCompaction := func(routed bool) error {
		s := settings.Load()
		return codexAsyncSync(path, at.key("codex.async"), routed, s.CodexAsyncCompact, s.CodexAsyncCompactPercent)
	}
	// keptBearerToken is the table's bearer token when it is one the
	// gateway takes: a caller key of the gateway's own the user put in
	// agentlink's token's place goes through agentlink all the same, so theirs
	// stays, as a sync keeps one (keyHolds); only a value the gateway would
	// refuse is wired over (#2545). The sharing key agentlink made is agentlink's
	// own, however much Authenticate takes it, so it never counts as theirs
	// (Default): sharing going off brings gateway.Token back
	keptBearerToken := func(table string) string {
		tok := at.gwKey()
		if t, err := edit.GetTOMLTable(path, table); err == nil {
			if v := t["experimental_bearer_token"]; v != tok && keyHolds(v, tok) && !access.Default(v) {
				return v
			}
		}
		return tok
	}
	putProvider := func() error {
		kvs := []edit.KV{
			{Path: "name", Value: "agentlink"},
			{Path: "base_url", Value: at.v1()},
			{Path: "wire_api", Value: "responses"},
			{Path: "experimental_bearer_token", Value: keptBearerToken("model_providers." + agentlinkID)},
			// Codex offers its built-in image tool (image_gen.imagegen) and
			// the other OpenAI-provider tools only to a provider it reads as
			// the OpenAI actor: one that does not require OpenAI auth but
			// carries x-openai-actor-authorization. Without this header the
			// model is never given the tool, so a Codex on agentlink can't draw
			// (the request that names model/draws never happens at all).
			// uses_openai_actor_authorization() is exactly this check.
			// An Inline table: a config that holds the headers as their own
			// [model_providers.agentlink.http_headers] table, or as dotted keys,
			// gets the header set there, the user's others kept; written
			// inline beside that table it defined http_headers twice and
			// every model switch failed (wztlink1013 on Discord).
			{Path: "http_headers", Value: edit.Inline{{Path: "x-openai-actor-authorization", Value: "agentlink"}}},
		}
		// the table's capabilities, the user's or the background
		// compaction's, aren't lost as it is written again
		if caps := codexProviderCaps(path); caps != nil {
			kvs = append(kvs, edit.KV{Path: "capabilities", Value: caps})
		}
		if err := edit.SetTOMLTable(path, "model_providers."+agentlinkID, kvs...); err != nil {
			return err
		}
		return asyncCompaction(true)
	}
	hasProvider := func() bool {
		t, err := edit.GetTOMLTable(path, "model_providers."+agentlinkID)
		return err == nil && t != nil
	}
	// noProvider: the config was read and has no such table; one agentlink
	// can't read is told by Check as it is, and written to by nothing
	noProvider := func() bool {
		t, err := edit.GetTOMLTable(path, "model_providers."+agentlinkID)
		return err == nil && t == nil
	}
	// namesAgentLink: the config has Codex on agentlink as its provider, at its
	// top or in a profile. Codex then reads [model_providers.agentlink] as it
	// loads the config, before any thread: without the table it loads no
	// config at all ("Model provider `agentlink` not found"), the app and the
	// CLI both, whatever the model.
	namesAgentLink := func() bool {
		if asProvider() {
			return true
		}
		names, _ := edit.TOMLTables(path)
		for _, n := range names {
			if strings.HasPrefix(n, "profiles.") {
				if t, _ := edit.GetTOMLTable(path, n); t["model_provider"] == agentlinkID {
					return true
				}
			}
		}
		return false
	}
	// dropProvider takes agentlink out as the provider Codex is on; the table
	// stays for the threads started on it
	dropProvider := func() error {
		if !asProvider() {
			return nil
		}
		if err := edit.DelTOMLTop(path, "model_provider", "model_catalog_json"); err != nil {
			return err
		}
		dropList()
		return nil
	}
	// mirrorsTaken are the mirrors takeMirrors pointed at agentlink (#1796)
	mirrorsTaken := func() []string {
		var ids []string
		json.Unmarshal([]byte(stashLoad()[at.key("codex.mirrors")]), &ids)
		return ids
	}
	// isCCSwitchMirror reports whether a model_providers table is CC Switch's
	// official OpenAI mirror: name = "OpenAI", requires_openai_auth = true, and no
	// base_url (or agentlink's codexURL while failing over, or while
	// takeMirrors has it).
	isCCSwitchMirror := func(p string) bool {
		if p == "" || p == "openai" || p == agentlinkID {
			return false
		}
		t, err := edit.GetTOMLTable(path, "model_providers."+p)
		if err != nil || t["name"] != "OpenAI" || t["requires_openai_auth"] != "true" {
			return false
		}
		u := t["base_url"]
		return u == "" || (u == at.codexURL() && (stashLoad()[at.key("codex.mirror_failover")] == p || slices.Contains(mirrorsTaken(), p)))
	}
	// profileNamed are the provider tables a profile of config.toml names:
	// the user's to pick, each left as it is
	profileNamed := func(names []string) map[string]bool {
		named := map[string]bool{}
		for _, n := range names {
			if strings.HasPrefix(n, "profiles.") {
				if t, _ := edit.GetTOMLTable(path, n); t["model_provider"] != "" {
					named[t["model_provider"]] = true
				}
			}
		}
		return named
	}
	// CC Switch's provider tables in Codex's config, each one's base URL
	// by its id. A thread keeps the provider it was
	// started on, and CC Switch moves every third-party thread onto its
	// "custom" one, so one reopened there still went to the relay, a
	// agentlink model picked in it too, which the relay didn't know. While a
	// agentlink model is on, those tables go through agentlink as well; a table
	// a profile names is the user's to pick, and stays as it is.
	ccSwitchTables := func() map[string]string {
		names, _ := edit.TOMLTables(path)
		named := profileNamed(names)
		out := map[string]string{}
		for _, n := range names {
			id, ok := strings.CutPrefix(n, "model_providers.")
			if !ok || !ccSwitchProvider.MatchString(id) || named[id] || isCCSwitchMirror(id) {
				continue
			}
			if t, _ := edit.GetTOMLTable(path, n); t["base_url"] != "" {
				out[id] = t["base_url"]
			}
		}
		return out
	}
	// putThreadTables writes a table for each provider a Codex thread was
	// started on that config.toml has none of, pointed at agentlink. The
	// Codex app resumes a thread on its own provider, and without that
	// table it loads no config for it: "Model provider `custom` not found",
	// a thread started through CC Switch reopened after CC Switch rewrote
	// config.toml without its table (#1372). Codex's built-in providers
	// need none, and agentlink's own is putProvider's. A table written so
	// stays, as agentlink's own does, for the threads that name it.
	putThreadTables := func() error {
		// the providers config.toml has, however spelled (a table, inline,
		// dotted keys); one that doesn't parse is left alone
		raw, err := edit.Read(path)
		if err != nil {
			return nil
		}
		var cfg struct {
			Providers map[string]any `toml:"model_providers"`
		}
		if toml.Unmarshal(raw, &cfg) != nil {
			return nil
		}
		for _, id := range sessions.CodexThreadProviders(dir) {
			if _, ok := cfg.Providers[id]; ok || id == agentlinkID || codexBuiltinProvider[id] {
				continue
			}
			if err := edit.SetTOMLTable(path, "model_providers."+id,
				edit.KV{Path: "name", Value: id},
				edit.KV{Path: "base_url", Value: at.v1()},
				edit.KV{Path: "wire_api", Value: "responses"},
				edit.KV{Path: "experimental_bearer_token", Value: keptBearerToken("model_providers." + id)},
			); err != nil {
				return err
			}
		}
		return nil
	}
	// takeMirrors points CC Switch's OpenAI mirrors (the official OpenAI on
	// the ChatGPT sign-in, under an id of its own and with no base_url) at
	// agentlink's ChatGPT backend while Codex's built-in provider goes there
	// by openai_base_url, which a table of another id doesn't follow. A
	// thread started on one ("custom") and reopened went straight to
	// ChatGPT, which refused the agentlink model picked in it ("The
	// 'group/intent-auto' model is not supported when using Codex with a
	// ChatGPT account"), and agentlink never saw the request (#1796). Through
	// agentlink it is answered as the built-in provider's threads are: a
	// agentlink model by agentlink, Codex's own on the ChatGPT accounts. A table
	// a profile names stays as it is.
	takeMirrors := func() error {
		if !viaBase() {
			return nil
		}
		names, _ := edit.TOMLTables(path)
		named := profileNamed(names)
		taken := mirrorsTaken()
		for _, n := range names {
			id, ok := strings.CutPrefix(n, "model_providers.")
			if !ok || named[id] || !isCCSwitchMirror(id) {
				continue
			}
			if t, _ := edit.GetTOMLTable(path, n); t["base_url"] != "" {
				continue
			}
			if err := edit.SetTOMLKey(path, n, "base_url", at.codexURL()); err != nil {
				return err
			}
			if !slices.Contains(taken, id) {
				taken = append(taken, id)
			}
		}
		if len(taken) == 0 {
			return nil
		}
		b, _ := json.Marshal(taken)
		stash(map[string]string{at.key("codex.mirrors"): string(b)})
		return nil
	}
	// giveMirrors takes the base URL takeMirrors wrote off again; one
	// changed since is left as it is now
	giveMirrors := func() error {
		taken := mirrorsTaken()
		if len(taken) == 0 {
			return nil
		}
		for _, id := range taken {
			t, err := edit.GetTOMLTable(path, "model_providers."+id)
			if err != nil || t["base_url"] != at.codexURL() || stashLoad()[at.key("codex.mirror_failover")] == id {
				continue
			}
			if err := edit.DelTOMLKey(path, "model_providers."+id, "base_url"); err != nil {
				return err
			}
		}
		forget(at.key("codex.mirrors"))
		return nil
	}
	// takeTables points CC Switch's tables at agentlink, each one's own base
	// URL kept in the stash for giveTables (a table already on agentlink's
	// keeps the one kept for it), and writes the tables threads name that
	// are gone (putThreadTables), and takes CC Switch's OpenAI mirrors
	// (takeMirrors)
	takeTables := func() error {
		if err := putThreadTables(); err != nil {
			return err
		}
		if err := takeMirrors(); err != nil {
			return err
		}
		was := map[string]string{}
		json.Unmarshal([]byte(stashLoad()[at.key("codex.tables")]), &was)
		for id, u := range ccSwitchTables() {
			if u == at.v1() {
				continue
			}
			if err := edit.SetTOMLKey(path, "model_providers."+id, "base_url", at.v1()); err != nil {
				return err
			}
			was[id] = u
		}
		if len(was) == 0 {
			return nil
		}
		b, _ := json.Marshal(was)
		stash(map[string]string{at.key("codex.tables"): string(b)})
		return nil
	}
	// giveTables puts the base URLs back, on the tables still pointed at
	// agentlink; one changed since is left as it is now
	giveTables := func() error {
		if err := giveMirrors(); err != nil {
			return err
		}
		var was map[string]string
		json.Unmarshal([]byte(stashLoad()[at.key("codex.tables")]), &was)
		now := ccSwitchTables()
		for id, u := range was {
			if now[id] != at.v1() {
				continue
			}
			if err := edit.SetTOMLKey(path, "model_providers."+id, "base_url", u); err != nil {
				return err
			}
		}
		forget(at.key("codex.tables"))
		return nil
	}
	// api: the user wants agentlink as Codex's provider even while Codex is
	// signed in to ChatGPT
	api := func() bool { return stashLoad()[at.key("codex.login")] == "api" }
	// out reports whether the Codex app holds the ChatGPT account Codex is
	// signed in to (codexUsedUp), for agentlink to become Codex's provider
	// while it does (#540): never with the sign-in kept on ChatGPT
	// (chatgpt), where the user would rather the Codex app wait for the
	// account than leave its ChatGPT state
	out := func() bool { return stashLoad()[at.key("codex.login")] != "chatgpt" && codexUsedUp() }
	dropBase := func() error {
		forget(at.key("codex.failover"))
		if !viaBase() {
			return nil
		}
		return edit.DelTOMLTop(path, "openai_base_url")
	}
	dropMirrorFailover := func() error {
		p := stashLoad()[at.key("codex.mirror_failover")]
		if p == "" {
			return nil
		}
		forget(at.key("codex.mirror_failover"))
		t, _ := edit.GetTOMLTable(path, "model_providers."+p)
		if t["base_url"] == at.codexURL() {
			return edit.DelTOMLKey(path, "model_providers."+p, "base_url")
		}
		return nil
	}
	// the model spawned subagents start on, when not the parent's; one of
	// agentlink's goes when agentlink steps out, as Codex could no longer find it
	subagent := func() (string, error) {
		agents, err := edit.GetTOMLTable(path, "agents")
		return agents["default_subagent_model"], err
	}
	// the model Codex writes its memories with (Yc on Discord): [memories]
	// extract_model summarises a thread, consolidation_model folds them in
	// (codex-rs/config/src/types.rs). Unset, Codex takes gpt-5.6-luna and
	// gpt-5.6-terra, whatever its model; agentlink sets both to one pick.
	memories := func() (string, error) {
		m, err := edit.GetTOMLTable(path, "memories")
		return cmp.Or(m["consolidation_model"], m["extract_model"]), err
	}
	dropMemories := func() error {
		for _, k := range []string{"extract_model", "consolidation_model"} {
			if err := edit.DelTOMLKey(path, "memories", k); err != nil {
				return err
			}
		}
		return nil
	}
	// one of agentlink's goes too: Codex could no longer find it
	dropSubagent := func() error {
		if m, err := memories(); err != nil {
			return err
		} else if isAgentLink(m) {
			if err := dropMemories(); err != nil {
				return err
			}
		}
		model, err := subagent()
		if err != nil {
			return err
		}
		if !isAgentLink(model) {
			return nil
		}
		return edit.DelTOMLKey(path, "agents", "default_subagent_model")
	}
	// the effort spawned subagents start at, when not the parent's
	// (core/src/agent/child_config.rs): checked against the model they run
	// on, theirs or the parent's, and a spawn at one it lacks fails, so one
	// that model doesn't take goes
	subEffort := func() (string, error) {
		agents, err := edit.GetTOMLTable(path, "agents")
		return agents["default_subagent_reasoning_effort"], err
	}
	subModel := func() string {
		if m, _ := subagent(); m != "" {
			return m
		}
		return get("model")
	}
	subEfforts := func() []string { return catalog.Efforts(models(), subModel()) }
	dropSubEffort = func() error {
		e, err := subEffort()
		if err != nil || e == "" {
			return err
		}
		if l := subEfforts(); len(l) > 0 && !contains(l, e) {
			return edit.DelTOMLKey(path, "agents", "default_subagent_reasoning_effort")
		}
		return nil
	}
	// the user disconnected Codex and hasn't connected it since: agentlink
	// writes nothing of its own back (paynezhuang on Discord: the base URL
	// was back after every update)
	disconnected := func() bool { return stashLoad()[at.key("codex"+disconnectedKey)] == "1" }
	// Codex on one of its own models goes through agentlink too while more of
	// its ChatGPT accounts are on there, so one out of its allowance hands
	// the turn to the next; with none, or once the user disconnected Codex,
	// it goes straight to OpenAI again
	failover := func() error {
		if wired() || asProvider() {
			return nil
		}
		p := get("model_provider")
		mirror := isCCSwitchMirror(p)
		if !mirror {
			_ = dropMirrorFailover()
		}
		if p != "" && p != "openai" && !mirror {
			return nil
		}
		// a Codex that reaches the gateway from another machine is never
		// pointed at the Codex path: the gateway takes a named key there,
		// so the turn failover handed it came back a 401 (#2145)
		on := !disconnected() && codexFailover() && !onAnotherMachine(at.gw())
		// failover is agentlink's own idea, not the user's: never handed
		// Codex's requests while a proxy they'd go through doesn't pass
		// them on to agentlink, which would have every turn fail with 502
		// (codex_proxy.go); one written before goes again
		if on && at.spell == nil && !codexProxyOK(path, at.gw()) {
			on = false
		}
		if mirror {
			t, _ := edit.GetTOMLTable(path, "model_providers."+p)
			hasGateway := t["base_url"] == at.codexURL()
			switch {
			case on && !hasGateway:
				if err := edit.SetTOMLKey(path, "model_providers."+p, "base_url", at.codexURL()); err != nil {
					return err
				}
				stash(map[string]string{at.key("codex.mirror_failover"): p})
			case !on && hasGateway:
				if err := edit.DelTOMLKey(path, "model_providers."+p, "base_url"); err != nil {
					return err
				}
				forget(at.key("codex.mirror_failover"))
			}
			return nil
		}
		// the base URL failover wrote is marked (codex.failover), and only
		// that one goes again: one the user wrote in config.toml by hand,
		// to have Codex's own models go through agentlink, stays (#856: it was
		// gone again each time agentlink started). One already there while
		// failover is on is taken as failover's, as a agentlink from before
		// the mark wrote it.
		owned := stashLoad()[at.key("codex.failover")] == "1"
		switch {
		case on && !viaBase():
			if err := edit.SetTOMLTop(path, edit.KV{Path: "openai_base_url", Value: at.codexURL()}); err != nil {
				return err
			}
			stash(map[string]string{at.key("codex.failover"): "1"})
		case on && !owned:
			stash(map[string]string{at.key("codex.failover"): "1"})
		case !on && viaBase() && owned:
			return dropBase()
		}
		return nil
	}
	// failingOver: Codex reaches agentlink for account failover alone, by the
	// base URL failover wrote (codex.failover) or by CC Switch's OpenAI
	// mirror pointed at agentlink for it (codex.mirror_failover), with nothing
	// of agentlink's connected. It read as not connected while every request
	// went through agentlink, agentlink's models in its list (#1385).
	failingOver := func() bool {
		if wired() || asProvider() {
			return false
		}
		s := stashLoad()
		if p := get("model_provider"); p != "" && p != "openai" {
			t, _ := edit.GetTOMLTable(path, "model_providers."+p)
			return s[at.key("codex.mirror_failover")] == p && t["base_url"] == at.codexURL()
		}
		return viaBase() && s[at.key("codex.failover")] == "1"
	}
	modelOptions := func(withAgentLink bool) []Option {
		var own []Option
		// each of Codex's own says which way it goes, as Claude Code's do:
		// to its provider directly, or through agentlink while routed by the
		// base URL or with agentlink its provider (EZN7L2C3, #834: Codex 下拉
		// 选项中没有 via agentlink)
		p := get("model_provider")
		to, through := "OpenAI", routed()
		if p != "" && p != agentlinkID && p != "openai" {
			// a provider of the user's own, or CC Switch's, pointed at
			// agentlink or not
			t, _ := edit.GetTOMLTable(path, "model_providers."+p)
			to, through = p, t["base_url"] == at.codexURL() || t["base_url"] == at.v1()
		}
		say := func(own []Option) []Option {
			for i := range own {
				if through {
					own[i].Via = true
				} else {
					own[i].Direct = to
				}
			}
			return own
		}
		if p != "" && p != agentlinkID && !isCCSwitchMirror(p) {
			own = say(group(p, options(catalog.Codex(), "")))
		} else {
			own = say(group("OpenAI", options(ownCodex(), "")))
			// with agentlink Codex's provider (agentlink API, or a agentlink model
			// picked with no ChatGPT sign-in beside it), Codex's own models
			// are reached through agentlink on a ChatGPT account there: picked
			// so, set so (#701). One agentlink doesn't serve is picked off
			// agentlink, straight to OpenAI, and says so: it said via agentlink,
			// and picking it took Codex off agentlink at once (#1269)
			if api() || asProvider() {
				for i, o := range own {
					if id := codexOwnViaAgentLink(o.Value); id != "" {
						own[i].Label, own[i].Value, own[i].Ref = o.Value, id, id
					} else if !api() {
						own[i].Via, own[i].Direct = false, "OpenAI"
					}
				}
			}
		}
		if !withAgentLink {
			return own
		}
		// the models agentlink serves on the Codex accounts in it (codex/…)
		// are left out while they would only add a hop: Codex on its own
		// ChatGPT sign-in, that one account alone in agentlink. On a relay or
		// a key they are not Codex's sign-in, and over several accounts
		// agentlink routes; offered via agentlink then, beside the direct ones
		// (#1603: Codex on CC Switch's custom relay, two accounts, GPT
		// models direct only). Routed, Codex's own say via agentlink already.
		onSignIn := (p == "" || p == "openai" || isCCSwitchMirror(p)) && chatGPT()
		if !through && (!onSignIn || codexFailover()) {
			return append(own, viaAgentLink("codex", "")...)
		}
		return append(own, viaAgentLinkFor("codex", "")...)
	}
	// unroute takes agentlink out as the way Codex reaches its models and puts
	// back what the stash kept from before agentlink was wired in (its
	// provider, catalog and effort); it answers the model Codex was on
	// then, for Unwire to go back to
	unroute := func() (string, error) {
		forget(at.key("codex.out"), at.key("codex.joined"), at.key("codex.beside"))
		if err := giveTables(); err != nil {
			return "", err
		}
		if !routed() {
			return "", nil
		}
		if err := dropSubagent(); err != nil {
			return "", err
		}
		if err := dropBase(); err != nil {
			return "", err
		}
		if err := dropProvider(); err != nil {
			return "", err
		}
		was := unstash(at.key("codex.model"))
		var back []edit.KV
		if p := unstash(at.key("codex.provider")); p != "" && p != agentlinkID {
			back = append(back, edit.KV{Path: "model_provider", Value: p})
		}
		if c := unstash(at.key("codex.catalog")); c != "" && !ownCatalog(c) {
			back = append(back, edit.KV{Path: "model_catalog_json", Value: c})
		}
		if e := unstash(at.key("codex.effort")); e != "" {
			back = append(back, edit.KV{Path: "model_reasoning_effort", Value: e})
		}
		if len(back) > 0 {
			if err := edit.SetTOMLTop(path, back...); err != nil {
				return "", err
			}
		}
		return was, nil
	}
	set := func(v string) error {
		if v == "" {
			if err := dropSubagent(); err != nil {
				return err
			}
			if err := giveTables(); err != nil {
				return err
			}
			if err := dropMirrorFailover(); err != nil {
				return err
			}
			// Codex as installed: OpenAI, its own catalog, its default model
			if err := dropBase(); err != nil {
				return err
			}
			if err := edit.DelTOMLTop(path, "model", "model_provider", "model_catalog_json"); err != nil {
				return err
			}
			dropList()
			forget(at.key("codex.model"), at.key("codex.effort"), at.key("codex.provider"), at.key("codex.catalog"), at.key("codex.out"), at.key("codex.joined"), at.key("codex.beside"))
			return nil
		}
		// agentlink API: agentlink stays Codex's provider whichever model is the
		// default, so one of Codex's own goes as agentlink serves it, on the
		// ChatGPT account there; taking agentlink out for it took agentlink's
		// models out of Codex's picker with it (#701)
		if api() && !isAgentLink(v) {
			id := codexOwnViaAgentLink(v)
			if id == "" {
				return fmt.Errorf("Codex's sign-in is agentlink API, and agentlink serves no %s: switch Codex's ChatGPT subscription on in agentlink, or set Codex's sign-in to ChatGPT", v)
			}
			v = id
		}
		if isAgentLink(v) {
			if err := dropMirrorFailover(); err != nil {
				return err
			}
			if !routed() {
				stash(map[string]string{at.key("codex.model"): get("model"), at.key("codex.effort"): get("model_reasoning_effort"),
					at.key("codex.provider"): get("model_provider"), at.key("codex.catalog"): get("model_catalog_json")})
			}
			// a ChatGPT account the Codex app holds (out of allowance and
			// of credits) keeps it from sending at all, a agentlink model's
			// request too; as a provider of Codex's own, agentlink is past
			// that. Wired so for that alone, it is marked (codex.out), for
			// Sync to put it back beside the sign-in once the account has
			// room again.
			chatgpt := !api() && chatGPT()
			if chatgpt && !out() {
				forget(at.key("codex.out"))
				if err := dropProvider(); err != nil {
					return err
				}
				// for the threads started while agentlink was the provider
				if err := putProvider(); err != nil {
					return err
				}
				// the base URL is the built-in provider's, and a catalog
				// file would stand in for the list agentlink hands out
				if err := edit.DelTOMLTop(path, "model_provider", "model_catalog_json"); err != nil {
					return err
				}
				if err := edit.SetTOMLTop(path,
					edit.KV{Path: "openai_base_url", Value: at.codexURL()},
					edit.KV{Path: "model", Value: v},
				); err != nil {
					return err
				}
				stash(map[string]string{at.key("codex.beside"): "1"})
				if err := takeTables(); err != nil {
					return err
				}
				return settle()
			}
			if chatgpt {
				stash(map[string]string{at.key("codex.out"): "1"})
			} else {
				forget(at.key("codex.out"))
			}
			// agentlink is its provider now, not beside the sign-in
			forget(at.key("codex.beside"))
			if err := putProvider(); err != nil {
				return err
			}
			if err := writeOwnList(catalogPath, ownList()); err != nil {
				return err
			}
			// a thread started on Codex's built-in provider stays on it when
			// opened again, and its model can be switched to one of agentlink's
			// there (the Codex app's, ChatGPT Desktop's picker): the base
			// URL sends that one to agentlink too, not to the ChatGPT backend,
			// which refuses it (#259). A base URL of the user's own stays.
			kv := []edit.KV{
				{Path: "model_provider", Value: agentlinkID},
				{Path: "model_catalog_json", Value: catalogRef},
				{Path: "model", Value: v},
			}
			if u := get("openai_base_url"); u == "" || viaBase() {
				kv = append(kv, edit.KV{Path: "openai_base_url", Value: at.codexURL()})
			}
			if err := edit.SetTOMLTop(path, kv...); err != nil {
				return err
			}
			if err := takeTables(); err != nil {
				return err
			}
			return settle()
		}
		// joined, a model of Codex's own is picked beside agentlink's, as in
		// Codex's /model: it stays connected
		if joined() && !api() && chatGPT() {
			if err := edit.SetTOMLTop(path, edit.KV{Path: "model", Value: v}); err != nil {
				return err
			}
			return settle()
		}
		if _, err := unroute(); err != nil {
			return err
		}
		if err := edit.SetTOMLTop(path, edit.KV{Path: "model", Value: v}); err != nil {
			return err
		}
		if err := failover(); err != nil {
			return err
		}
		return settle()
	}

	a := atomic(&Agent{
		ID: "codex", Name: "Codex", Icon: "codex-color", Bin: "codex", Dir: dir, Path: path,
		UA: []string{"codex", "acp-extension-codex"}, // The ACP adapter's default client name becomes Codex's originator.
		// Codex as it was before agentlink: its default puts it back as
		// installed, OpenAI and its default model, where this brings back
		// the provider and model the user had
		Unwire: func() error {
			now := get("model")
			was, err := unroute()
			if err != nil {
				return err
			}
			if err := dropSubagent(); err != nil {
				return err
			}
			// the model left alone where agentlink had none to take over; a
			// model of Codex's own picked while joined stays, its last pick
			switch {
			case now != "" && !isAgentLink(now):
			case was != "" && !isAgentLink(was):
				err = edit.SetTOMLTop(path, edit.KV{Path: "model", Value: was})
			case isAgentLink(get("model")):
				err = edit.DelTOMLTop(path, "model")
			}
			if err != nil {
				return err
			}
			dropList()
			forget(at.key("codex.model"), at.key("codex.effort"), at.key("codex.provider"), at.key("codex.catalog"))
			return settle()
		},
		// signed in with ChatGPT, Codex reaches agentlink by the base URL with
		// its own models still there: connected so, it stays on the model it
		// was on, the one last picked in its /model (the owner: 让 Codex 记住
		// 上次的选择), and agentlink's models join its list
		Join: func() (bool, error) {
			if api() || !chatGPT() || out() || isAgentLink(get("model")) {
				return false, nil
			}
			if p := get("model_provider"); p != "" && p != "openai" && !isCCSwitchMirror(p) {
				return false, nil
			}
			if err := dropMirrorFailover(); err != nil {
				return false, err
			}
			if !routed() {
				stash(map[string]string{at.key("codex.model"): get("model"), at.key("codex.effort"): get("model_reasoning_effort"),
					at.key("codex.provider"): get("model_provider"), at.key("codex.catalog"): get("model_catalog_json")})
			}
			forget(at.key("codex.out"))
			if err := dropProvider(); err != nil {
				return false, err
			}
			// for the threads started on one of agentlink's models
			if err := putProvider(); err != nil {
				return false, err
			}
			if err := edit.DelTOMLTop(path, "model_provider", "model_catalog_json"); err != nil {
				return false, err
			}
			if err := edit.SetTOMLTop(path, edit.KV{Path: "openai_base_url", Value: at.codexURL()}); err != nil {
				return false, err
			}
			stash(map[string]string{at.key("codex.joined"): "1"})
			if err := takeTables(); err != nil {
				return false, err
			}
			if err := codexStaleCache(filepath.Join(dir, "models_cache.json"), provider.CodexListTag()); err != nil {
				return false, err
			}
			return true, settle()
		},
		// joined as the GUI and drift tell it: Codex's own models in its
		// list beside agentlink's. With agentlink its provider (agentlink API, or a
		// ChatGPT account the Codex app holds) they are not, though the
		// mark stays for Sync to put it back beside the sign-in: the
		// Agents row said "its own models stay in /model" while the Codex
		// app was meant to be in its API state (#2267)
		Joined:      func() bool { return joined() && !asProvider() },
		Beside:      beside,
		Routed:      routed,
		FailingOver: failingOver,
		EndFailover: func() error {
			if !failingOver() {
				return nil
			}
			return errors.Join(dropMirrorFailover(), dropBase())
		},
		OwnVia: codexOwnViaAgentLink,
		Sync: func() error {
			if err := asyncCompaction(routed()); err != nil {
				return err
			}
			// model_provider = "agentlink" left with its table gone (taken by
			// another tool, or a hand edit), which keeps Codex from loading
			// its config at all, connected to agentlink or not (Tystem on
			// Discord): the table is written back, as it stays once written
			if namesAgentLink() && noProvider() {
				if err := putProvider(); err != nil {
					return err
				}
			}
			if err := failover(); err != nil {
				return err
			}
			// agentlink API on one of Codex's own models, as an older agentlink
			// left it unwired: wired again, as picking it does now (#701);
			// not one the user disconnected, which this left the same
			if m := get("model"); api() && !disconnected() && !isAgentLink(m) && !asProvider() && codexOwnViaAgentLink(m) != "" {
				return set(m)
			}
			// on a agentlink model by the base URL alone with no ChatGPT
			// sign-in, as an older agentlink left a Codex signed in with an
			// API key: its picker never had agentlink's models (#322)
			if m := get("model"); isAgentLink(m) && viaBase() && get("model_provider") == "" && !chatGPT() {
				return set(m)
			}
			// the ChatGPT account ran out (of its allowance and its
			// credits) after a agentlink model was picked beside its sign-in:
			// the Codex app then sends nothing, a agentlink model's turn
			// included, in a new thread or an old one (#540), so agentlink
			// becomes Codex's provider as set does for an account already
			// out; and once the account has room again (or Codex is on
			// one with room), Codex's own models join agentlink's again
			if m := get("model"); isAgentLink(m) && !api() && chatGPT() {
				switch {
				case viaBase() && !asProvider() && out():
					return set(m)
				case asProvider() && stashLoad()[at.key("codex.out")] == "1" && !out():
					return set(m)
				}
			}
			// a table taken away before (by an older agentlink) comes back
			// while agentlink is wired, for the threads that name it
			if wired() && viaBase() && !hasProvider() {
				if err := putProvider(); err != nil {
					return err
				}
			}
			// CC Switch's tables, left from before agentlink took them over
			if wired() && routed() {
				if err := takeTables(); err != nil {
					return err
				}
			}
			switch {
			case asProvider() && ownCatalog(get("model_catalog_json")):
				// set up by a agentlink from before #259: the threads started
				// on Codex's built-in provider reach agentlink too
				if get("openai_base_url") == "" {
					if err := edit.SetTOMLTop(path, edit.KV{Path: "openai_base_url", Value: at.codexURL()}); err != nil {
						return err
					}
				}
				b := ownList()
				if cur, _ := edit.Read(catalogPath); string(cur) != string(b) {
					if err := writeOwnList(catalogPath, b); err != nil {
						return err
					}
				}
			case failingOver():
				// the list the gateway hands a Codex there for failover
				// alone: its own models, without agentlink's (#1385)
				return codexStaleCache(filepath.Join(dir, "models_cache.json"), provider.CodexOwnListTag())
			case viaBase():
				if err := codexStaleCache(filepath.Join(dir, "models_cache.json"), provider.CodexListTag()); err != nil {
					return err
				}
			default:
				return nil
			}
			// a model that now has levels it had none of before (models.dev
			// synced, a vendor's list fetched) keeps an effort it takes
			if isAgentLink(get("model")) {
				return settle()
			}
			// on a model of Codex's own (the app's last pick), the app's
			// picker still lists agentlink's: the efforts they take now are
			// offered there too. A group that came to take max kept only
			// high in the app, with no control, while the config was on
			// Codex's own model (#2127)
			if routed() {
				return codexEnableEfforts(path, agentlinkModels("codex"))
			}
			return nil
		},
		Check: func() string {
			if namesAgentLink() && noProvider() {
				return "Codex's config names agentlink as its model_provider but has no [model_providers.agentlink], so Codex can't load its config (\"Model provider `agentlink` not found\"): agentlink writes it back when it next syncs, or connect Codex again"
			}
			if !wired() {
				return ""
			}
			// a profile's settings win over the top level's, agentlink's included
			if p := get("profile"); p != "" {
				t, err := edit.GetTOMLTable(path, "profiles."+p)
				if err != nil {
					return err.Error()
				}
				for _, k := range []string{"model", "model_provider", "openai_base_url", "model_catalog_json"} {
					if v, ok := t[k]; ok && v != get(k) {
						return "Codex's profile " + p + " sets its own " + k + " (" + v + "), which Codex takes over agentlink's"
					}
				}
			}
			switch {
			case asProvider():
				t, err := edit.GetTOMLTable(path, "model_providers."+agentlinkID)
				if err != nil {
					return err.Error()
				}
				if t["base_url"] != at.v1() || !keyHolds(t["experimental_bearer_token"], at.gwKey()) || t["wire_api"] != "responses" {
					return "Codex's [model_providers.agentlink] no longer points at agentlink's gateway (" + at.v1() + ")"
				}
				if c := get("model_catalog_json"); !ownCatalog(c) {
					return "Codex's model_catalog_json is no longer agentlink's list"
				}
				if _, err := os.Stat(catalogPath); err != nil {
					return "agentlink's model list for Codex (" + catalogPath + ") is gone"
				}
			case viaBase():
				if u := get("openai_base_url"); strings.TrimSuffix(u, "/") != at.codexURL() {
					return "Codex's openai_base_url is " + u + ", not agentlink's gateway at " + at.codexURL()
				}
			default:
				return "Codex's config no longer sends its model through agentlink (no openai_base_url or model_provider of agentlink's), so Codex asks OpenAI for a model OpenAI doesn't have"
			}
			for id, u := range ccSwitchTables() {
				if u != at.v1() {
					return "Codex's [model_providers." + id + "] (CC Switch's) sends to " + u + ", not agentlink's gateway: a Codex thread started on it goes there when reopened, a agentlink model picked in it too"
				}
			}
			return ""
		},
		// every prompt typed into Codex goes into history.jsonl
		LastUsed: func() time.Time { return lastJSONLTime(filepath.Join(dir, "history.jsonl"), "ts", "text") },
		// the Codex app writes no history.jsonl, but it and the TUI log each
		// model request they open
		Reached: func(since time.Time) (time.Time, string, bool) { return codexReached(dir, since) },
		// the app-server behind the Codex app (and every codex TUI) builds
		// its model list once, at start-up. Codex 0.162's TUI attaches to a
		// background app-server it starts once and leaves running (its
		// daemon_auto_start, on by default on every OS), so closing every
		// codex session keeps the old list: a new one shows Codex's own
		// models until that daemon is restarted, while the desktop app's
		// own app-server, restarted with the app, has agentlink's (TJHHHH,
		// luci). With no codex session on it agentlink restarts it itself
		// (codexDaemonNotice); with one, the user says when.
		Notice: func() string {
			var out []string
			if w := codexSubAgentsV1Warning(path, routed()); w != "" {
				out = append(out, w)
			}
			if at.spell == nil {
				if n, ok := codexDaemonNotice(path); ok {
					return strings.Join(append(out, n), " ")
				}
			}
			if codexRunning() {
				out = append(out, codexRestartAll)
			}
			return strings.Join(out, " ")
		},
		Fields: []Field{
			{
				Key: "model", Label: "model",
				Get:     func() string { return get("model") },
				Set:     set,
				Options: func(map[string]string) []Option { return modelOptions(true) },
			},
			{
				Key: "effort", Label: "effort",
				// unset reads as Default, which it is; the level Codex then
				// takes is on the Default option (Takes), not the value, so
				// picking Default doesn't read back as medium (lgtm)
				Get: func() string { return get("model_reasoning_effort") },
				// set, agentlink's models start at it too when Codex is switched
				// to one, or at their nearest (relist, codexcat.ModelDefault):
				// it fell back to the middle of the road (Creat.wu on Discord)
				Set: func(v string) error {
					var err error
					if v == "" {
						err = edit.DelTOMLTop(path, "model_reasoning_effort")
					} else {
						err = edit.SetTOMLTop(path, edit.KV{Path: "model_reasoning_effort", Value: v})
					}
					if err != nil {
						return err
					}
					return relist()
				},
				// Default first, with the level Codex takes for the model
				// when none is set, as the catalog entry it reads says it
				Options: func(cur map[string]string) []Option { return codexEfforts(models(), cur["model"]) },
			},
			{
				// Codex lists only the first five models in the spawn_agent
				// tool it gives the model, its own ahead of agentlink's (the
				// gateway names the rest there, spawnAgentModels), and a
				// subagent is put on one of agentlink's here without the lead
				// naming it. It is only the model a
				// spawn that names none starts on: unset, the lead's own,
				// but a lead naming another in spawn_agent gets that one
				// (core/src/agent/child_config.rs), so the GUI says the
				// lead picks rather than "same as model" (willz on Discord)
				Key: "subagent", Label: "subagents", Quiet: true,
				Get: func() string { v, _ := subagent(); return v },
				Set: func(v string) error {
					if v == "" {
						return edit.DelTOMLKey(path, "agents", "default_subagent_model")
					}
					if isAgentLink(v) && !routed() {
						return fmt.Errorf("pick a model through agentlink for Codex first; its subagents can then have one of their own")
					}
					if err := edit.SetTOMLKey(path, "agents", "default_subagent_model", v); err != nil {
						return err
					}
					return dropSubEffort()
				},
				Options: func(map[string]string) []Option { return modelOptions(routed()) },
			},
			{
				// the model agentlink puts every subagent on, whatever the
				// lead asked for in spawn_agent (willz on Discord): kept in
				// agentlink's settings, not Codex's config, since the gateway
				// rewrites the subagent's request. Every model agentlink serves
				// is offered (#1633: a lead on a chat provider, its
				// subagents wanted there too); a subagent whose task a
				// ChatGPT-backed lead sealed stays on the model asked for
				// unless the one set is a ChatGPT account's (the gateway)
				Key: "subagent_model", Label: "subagent model", Quiet: true,
				Get: provider.CodexSubagentModel,
				Set: provider.SetCodexSubagentModel,
				Options: func(map[string]string) []Option {
					var out, groups []Option
					for _, e := range provider.CodexSubagentModels() {
						if e.Group != "" {
							groups = append(groups, Option{Value: e.ID, Label: e.Name, Note: "routing group · via agentlink",
								Icon: e.Provider.Icon, Icons: e.Icons, Group: RoutingGroups, Ref: e.ID})
							continue
						}
						note := e.Provider.Name + " · via agentlink"
						if a := e.Provider.Account; a != nil && a.User != "" {
							note = a.User + " · via agentlink"
						}
						out = append(out, Option{Value: e.ID, Label: e.Name, Note: note, Icon: e.Provider.Icon,
							Group: e.Provider.Name, Ref: e.ID, Context: e.Context})
					}
					return append(out, groups...)
				},
			},
			{
				// [agents] default_subagent_reasoning_effort: unset, a
				// subagent runs at the session's effort, or at its model's
				// default when it has a model of its own
				Key: "subagent_effort", Label: "subagent effort", Quiet: true,
				Get: func() string { v, _ := subEffort(); return v },
				Set: func(v string) error {
					if v == "" {
						return edit.DelTOMLKey(path, "agents", "default_subagent_reasoning_effort")
					}
					if l := subEfforts(); len(l) > 0 && !contains(l, v) {
						return fmt.Errorf("Codex runs %s at %s, not %q: it would refuse to start a subagent", subModel(), strings.Join(l, ", "), v)
					}
					return edit.SetTOMLKey(path, "agents", "default_subagent_reasoning_effort", v)
				},
				Options: func(map[string]string) []Option {
					if e := subEfforts(); len(e) > 0 {
						return static(e...)
					}
					return static("low", "medium", "high", "xhigh")
				},
			},
			{
				// [memories] extract_model and consolidation_model, one
				// pick for both; Default removes them, for Codex's own
				Key: "memories", Label: "memories", Quiet: true,
				Get: func() string { v, _ := memories(); return v },
				Set: func(v string) error {
					if v == "" {
						return dropMemories()
					}
					if isAgentLink(v) && !routed() {
						return fmt.Errorf("pick a model through agentlink for Codex first; its memories can then be written with one of agentlink's")
					}
					for _, k := range []string{"extract_model", "consolidation_model"} {
						if err := edit.SetTOMLKey(path, "memories", k, v); err != nil {
							return err
						}
					}
					return nil
				},
				Options: func(map[string]string) []Option { return modelOptions(routed()) },
			},
			{
				// how Codex takes agentlink's models: beside its ChatGPT
				// sign-in (openai_base_url), agentlink its provider only while
				// the Codex app holds the account (""); beside it always,
				// held or not (chatgpt); or with agentlink as its provider,
				// the Codex app in its API state. Codex's getAuthStatus has
				// no token for a provider without requires_openai_auth, so
				// its Chrome control needs ChatGPT; the api note says so
				// (#1824). Kept in the stash, where set("") leaves it.
				Key: "login", Label: "sign-in", Quiet: true,
				Get: func() string { return stashLoad()[at.key("codex.login")] },
				Set: func(v string) error {
					if v != "" && v != "api" && v != "chatgpt" {
						return fmt.Errorf("sign-in is api, chatgpt or empty (ChatGPT), not %q", v)
					}
					stash(map[string]string{at.key("codex.login"): v})
					m := get("model")
					switch {
					case v != "api" && codexOwnOf(m) != "":
						// back beside the sign-in, Codex's own model goes to
						// OpenAI itself again, not by a hop through agentlink
						return set(codexOwnOf(m))
					case isAgentLink(m):
						return set(m)
					case v == "api" && codexOwnViaAgentLink(m) != "":
						// agentlink API on one of Codex's own models: wired as
						// for agentlink's, so its models are in Codex's picker
						return set(m)
					}
					return nil
				},
				Options: func(map[string]string) []Option {
					return []Option{
						{Value: "", Label: "ChatGPT", Note: "agentlink's models join Codex's own; Codex stays signed in to ChatGPT, and while the Codex app blocks its account (out of allowance, no credits left) agentlink is Codex's provider, so the app still sends"},
						{Value: "chatgpt", Label: "Always ChatGPT", Note: "as ChatGPT, and kept so while the Codex app blocks its account: agentlink never becomes Codex's provider. The Codex app may then send nothing till the account has room; Codex CLI goes on through agentlink"},
						{Value: "api", Label: "agentlink API", Note: "agentlink is Codex's provider; the Codex app is in its API state, with agentlink's models only. Codex's Chrome control needs the ChatGPT sign-in, so pick ChatGPT to keep it"},
					}
				},
			},
		},
	}, path, catalogPath)
	// this machine's Codex: the address its config keeps is tried (Drift),
	// and moved to WSL's new one when that changed (#1013)
	if at.spell == nil {
		a.reach = at.base
		a.viaProxy = func() (bool, bool, string, string) { return codexThroughProxy(path, at.gw()) }
		a.move = func(from, to string) error { return codexMoveGateway(path, from, to) }
	}
	return a
}

// ccSwitchProvider matches the ids of the provider tables CC Switch writes
// into Codex's config: "custom", and "cc-switch", "cc-switch-2"… from its
// older versions. Its "cc-switch-official" is its own proxy to OpenAI.
var ccSwitchProvider = regexp.MustCompile(`^(custom|cc-switch(-[0-9]+)?)$`)

// codexBuiltinProvider is the providers Codex has built in, which a thread
// can name with no table in config.toml (codex-rs model-provider-info:
// built_in_model_providers); "ollama-chat" is one it no longer has, and
// tells the user how to fix itself.
var codexBuiltinProvider = map[string]bool{
	"openai": true, "amazon-bedrock": true, "amazon-bedrock-runtime": true,
	"ollama": true, "lmstudio": true, "ollama-chat": true,
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// The reasoning efforts the Codex app knows, in its order (the enum of its
// enabled-reasoning-efforts setting), and those it offers while the
// setting is unset.
var (
	codexAppEfforts   = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "persistent"}
	codexAppEffortsOn = []string{"low", "medium", "high", "xhigh", "ultra", "persistent"}
)

// codexEnableEfforts adds the efforts agentlink's models take to the Codex
// app's [desktop] enabled-reasoning-efforts in config.toml: its model
// picker offers a model's efforts only when they are there, so a model's
// "max" or "minimal" was never offered (#310). The user's entries stay,
// only what is missing is added, and nothing is written when nothing is;
// an effort the app doesn't know is left out, as it would drop the whole
// setting for it. A [desktop] spelled some other way (inline, dotted
// keys) is left alone.
func codexEnableEfforts(path string, ms []catalog.Model) error {
	raw, err := edit.Read(path)
	if err != nil {
		return nil
	}
	var cfg struct {
		Desktop map[string]any `toml:"desktop"`
	}
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	on := codexAppEffortsOn
	if cfg.Desktop != nil {
		if tables, _ := edit.TOMLTables(path); !slices.Contains(tables, "desktop") {
			return nil
		}
		if v, ok := cfg.Desktop["enabled-reasoning-efforts"]; ok {
			xs, ok := v.([]any)
			if !ok {
				return nil
			}
			on = nil
			for _, x := range xs {
				s, ok := x.(string)
				if !ok {
					return nil
				}
				on = append(on, s)
			}
		}
	}
	add := false
	out := slices.Clone(on)
	for _, e := range codexAppEfforts {
		if slices.Contains(on, e) {
			continue
		}
		for _, m := range ms {
			if slices.Contains(m.Efforts, e) {
				out, add = append(out, e), true
				break
			}
		}
	}
	if !add {
		return nil
	}
	q := make([]string, len(out))
	for i, e := range out {
		q[i] = strconv.Quote(e)
	}
	return edit.SetTOMLKey(path, "desktop", "enabled-reasoning-efforts", edit.Raw("["+strings.Join(q, ", ")+"]"))
}

// ownCodex is Codex's own models, narrowed to the ones ticked on its ChatGPT
// subscription in agentlink when any are — the subscription switched off, its
// picks narrow nothing, as it serves no agent anything.
func ownCodex() []catalog.Model {
	ms := catalog.Codex()
	p, err := provider.Find("codex")
	if err != nil || p.Off || len(p.Models) == 0 {
		return ms
	}
	var out []catalog.Model
	for _, m := range ms {
		if slices.Contains(p.Models, m.ID) {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return ms
	}
	return out
}

// codexOwnViaAgentLink is the catalog id agentlink serves one of Codex's own
// models by on a ChatGPT account of the user's ("codex/gpt-5.5"), those
// shown to Codex first; "" when none serves it.
func codexOwnViaAgentLink(model string) string {
	if model == "" || isAgentLink(model) {
		return ""
	}
	shown, _ := provider.CatalogFor("codex")
	for _, es := range [][]provider.Entry{shown, provider.Catalog()} {
		for _, e := range es {
			if a := e.Provider.Account; e.Group == "" && e.Model == model && a != nil && a.Agent == "codex" {
				return e.ID
			}
		}
	}
	return ""
}

// codexOwnOf is the model of Codex's own a catalog id serves on a ChatGPT
// account (codexOwnViaAgentLink's way back); "" for any other.
func codexOwnOf(id string) string {
	if !isAgentLink(id) || strings.HasPrefix(id, provider.GroupPrefix) {
		return ""
	}
	p, model, ok := provider.Resolve(id)
	if !ok || p.Account == nil || p.Account.Agent != "codex" {
		return ""
	}
	return model
}

// codexGatewayURL is where Codex's built-in OpenAI provider is pointed to
// reach agentlink.
func codexGatewayURL() string { return gateway.URL() + gateway.CodexPath }

// codexKeptGateway is the gateway's URL for this machine's Codex: the
// address its config names when that is agentlink's gateway on a host other
// than this one's loopback (openai_base_url at the Codex path, or
// [model_providers.agentlink]'s base_url with agentlink's key, that one first
// while agentlink is Codex's provider), else gateway.URL(). It is http or
// https, on any port or the scheme's own, and may have a path before
// agentlink's: a company's reverse proxy at https://ai.corp.example/agentlink,
// or one on this machine at http://127.0.0.1:8788/<route> (#1985).
func codexKeptGateway(path string) string {
	kept := func(u, suffix string) string {
		base, ok := strings.CutSuffix(strings.TrimSuffix(strings.TrimSpace(u), "/"), suffix)
		if !ok || !onAnotherMachine(base) && !loopbackWithPath(base) {
			return ""
		}
		if p, err := url.Parse(base); err != nil || p.User != nil || p.RawQuery != "" || p.Fragment != "" {
			return ""
		}
		return base
	}
	base, _ := edit.GetTOMLTop(path, "openai_base_url")
	var table string
	if t, _ := edit.GetTOMLTable(path, "model_providers."+agentlinkID); ourKey(t["experimental_bearer_token"]) {
		table = kept(t["base_url"], "/v1")
	}
	// the one Codex is on first: agentlink's table when it is the provider
	if p, _ := edit.GetTOMLTop(path, "model_provider"); p == agentlinkID && table != "" {
		return table
	}
	return cmp.Or(kept(base, gateway.CodexPath), table, gateway.URL())
}

// codexMoveGateway points Codex's config at to where it names agentlink's
// gateway at from: openai_base_url and [model_providers.agentlink]'s
// base_url, each only when it is at from; anything else is the user's.
func codexMoveGateway(path, from, to string) error {
	from, to = strings.TrimSuffix(from, "/"), strings.TrimSuffix(to, "/")
	if u, _ := edit.GetTOMLTop(path, "openai_base_url"); strings.HasPrefix(u, from+"/") {
		if err := edit.SetTOMLTop(path, edit.KV{Path: "openai_base_url", Value: to + strings.TrimPrefix(u, from)}); err != nil {
			return err
		}
	}
	if t, _ := edit.GetTOMLTable(path, "model_providers."+agentlinkID); ourKey(t["experimental_bearer_token"]) && strings.HasPrefix(t["base_url"], from+"/") {
		return edit.SetTOMLKey(path, "model_providers."+agentlinkID, "base_url", to+strings.TrimPrefix(t["base_url"], from))
	}
	return nil
}

// isCodexGatewayOn reports whether an openai_base_url is agentlink's gateway
// reached at host, on whichever port it listened on then.
func isCodexGatewayOn(u, host string) bool {
	return strings.HasPrefix(u, "http://"+host+":") && strings.HasSuffix(strings.TrimSuffix(u, "/"), gateway.CodexPath)
}

// codexStaleCache ages Codex's cached model list if it isn't the one agentlink
// would hand out now (its ETag carries the list's tag), so the next Codex to
// start asks the gateway again rather than showing the old one until the
// cache ages on its own; the rest of the cache stays as Codex wrote it.
func codexStaleCache(path, tag string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c map[string]json.RawMessage
	if json.Unmarshal(b, &c) != nil {
		return nil
	}
	var etag string
	json.Unmarshal(c["etag"], &etag)
	if codexcat.Tagged(etag, tag) {
		return nil
	}
	old := time.Unix(0, 0).UTC().Format(time.RFC3339)
	var at string
	if json.Unmarshal(c["fetched_at"], &at) == nil && at == old {
		return nil
	}
	c["fetched_at"], _ = json.Marshal(old)
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return edit.WriteAtomic(path, out)
}

// codexUsedUp reports whether the ChatGPT account Codex is signed in to
// is held by the Codex app, out of its allowance with no credits to go on
// with (provider.CodexUsedUpKnown). A var so tests can say.
//
// What is known is used, never a reading asked for here: this runs from
// SyncCatalog, which a write's own request runs before it answers (a
// routing group's Save), and asking waits on the vendor — 1.1s of a
// user's Save. KeepOnAnAccountWithRoom asks on its own loop with the time
// for it, and tells the catalog when the answer changes.
var codexUsedUp = func() bool {
	return provider.CodexUsedUpKnown()
}

// codexFailover reports whether Codex is signed in to a ChatGPT account
// with more of its accounts on in agentlink, behind it, and the user hasn't
// asked agentlink to only switch its sign-in (provider.SwitchOnly).
func codexFailover() bool {
	// the user had agentlink only switch Codex's own sign-in (#1907)
	if provider.SwitchOnly("codex") {
		return false
	}
	for _, p := range provider.Accounts() {
		if p.Account != nil && p.Account.Agent == "codex" {
			return len(p.AlsoOn()) > 0
		}
	}
	return false
}

// EndCodexFailover takes account failover out of this machine's Codex
// as the agentlink serving its gateway quits (EndFailover): Codex goes
// straight to OpenAI again rather than retrying a gateway that's gone.
// The app-server codex sessions share read the old base URL as it started
// and keeps it, and KeepCodexDaemonCurrent ends with agentlink, so it is
// restarted here, when no session is on it (thisisvainxx on X: back on
// its own config, Codex took "several restarts").
func EndCodexFailover() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	a := codex(home)
	if !a.FailingOver() {
		return nil
	}
	if err := a.EndFailover(); err != nil {
		return err
	}
	codexDaemonCheck(a.Path)
	return nil
}

// CodexOwnOnly reports whether the Codex that asks the gateway for its
// model list there (/backend-api/codex/models) reaches agentlink for account
// failover alone (FailingOver), to be handed only its own models (#1385).
// That is this machine's Codex; a request doesn't say whether it came from
// a Codex in a WSL distro instead, so one there connected to agentlink (what
// agentlink set on it, or its joined/beside mark in the stash) keeps agentlink's
// models in the list. No distro is probed for it.
func CodexOwnOnly() bool {
	home, err := os.UserHomeDir()
	if err != nil || !codex(home).FailingOver() {
		return false
	}
	appliedMu.Lock()
	set := appliedLoad()
	appliedMu.Unlock()
	for id := range set {
		if strings.HasPrefix(id, "codex@wsl:") {
			return false
		}
	}
	// a distro's name may have dots of its own (Ubuntu-24.04)
	for k, v := range stashLoad() {
		if strings.HasPrefix(k, "codex@wsl:") && v == "1" && (strings.HasSuffix(k, ".joined") || strings.HasSuffix(k, ".beside")) {
			return false
		}
	}
	return true
}

// codexChatGPT reports whether Codex is signed in to a ChatGPT account:
// its built-in provider then runs and asks for the model list at
// openai_base_url. Signed in with an API key alone, it asks for none.
func codexChatGPT(dir string) bool {
	if settings.AccountDiscoveryOff() {
		return false
	}
	var a struct {
		Tokens struct {
			Access string `json:"access_token"`
		} `json:"tokens"`
	}
	// auth.json, or the macOS keychain item when Codex's config.toml
	// keeps the sign-in there (#2367)
	b, ok := provider.CodexSignIn(dir)
	if !ok || json.Unmarshal(b, &a) != nil {
		return false
	}
	return a.Tokens.Access != ""
}

// codexReached reads Codex's newest model request since a time — the
// newest day of it at most — from the log database the Codex app and the
// TUI share: the address it opened, and whether that was refused (nothing
// listening there). Zero when none is logged.
func codexReached(dir string, since time.Time) (at time.Time, to string, refused bool) {
	logs, _ := filepath.Glob(filepath.Join(dir, "logs_*.sqlite"))
	if len(logs) == 0 {
		return
	}
	slices.Sort(logs)
	db, err := provider.OpenReadOnly(logs[len(logs)-1])
	if err != nil {
		return
	}
	defer db.Close()
	from := max(since.Unix(), time.Now().Add(-24*time.Hour).Unix())
	rows, err := db.Query(`SELECT ts, feedback_log_body FROM logs
		WHERE ts > ? AND target = 'codex_api::endpoint::responses_websocket'
		ORDER BY ts DESC, ts_nanos DESC, id DESC LIMIT 20`, from)
	if err != nil {
		return
	}
	defer rows.Close()
	// newest first: a refusal comes before the attempt it answers
	failedAt := map[string]bool{}
	for rows.Next() {
		var ts int64
		var body string
		if rows.Scan(&ts, &body) != nil {
			continue
		}
		if _, u, ok := strings.Cut(body, "failed to connect to websocket: "); ok {
			if _, u, ok := strings.Cut(u, "url: "); ok && (strings.Contains(body, "Connection refused") || strings.Contains(body, "os error 10061")) {
				failedAt[strings.TrimSpace(u)] = true
			}
			continue
		}
		if _, u, ok := strings.Cut(body, "connecting to websocket: "); ok {
			u = strings.TrimSpace(u)
			return time.Unix(ts, 0), u, failedAt[u]
		}
	}
	return
}

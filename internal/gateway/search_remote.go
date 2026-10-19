package gateway

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/crazymsn/Agent-Link/internal/catalog"
	"github.com/crazymsn/Agent-Link/internal/provider"
)

// A Remote agentlink searches the web for the models it serves as it would
// for its own agents: by the model's provider, when that searches by
// itself, else with its own searcher. Its list says which of the two for
// each model (web_search: "native" or "agentlink"), and a web search offered
// to one of them goes to it as it was offered rather than being run here
// with this agentlink's searcher (Player on Discord: remoteAgentLink/glm-5.3's
// searches went to another model). Only a model it searches by itself,
// natively, is one this agentlink may search with for its other models: a
// search agentlink runs for another model is marked (SearchingHeader), and
// the other agentlink runs no searcher of its own for it, so two agentlinks
// that each name the other never ask each other in turn.

// SearchingHeader marks a request a agentlink made to search for another
// model, to a remote agentlink, which then searches only by the model itself.
const SearchingHeader = "X-AgentLink-Searching"

// Web search as a Remote agentlink's list says it.
const (
	searchNative = "native"
	searchAgentLink = "agentlink"
)

// remoteSearch is how a Remote agentlink says it searches the web for model,
// "" for another provider or a model it doesn't.
func remoteSearch(p provider.Provider, model string) string {
	if !p.IsRemoteAgentLink() {
		return ""
	}
	if i := slices.IndexFunc(p.Available(), func(m catalog.Model) bool { return m.ID == model }); i >= 0 {
		return p.Available()[i].WebSearch
	}
	return ""
}

// remoteSearchesAny is whether a Remote agentlink searches natively for any
// of its models, which makes it one this agentlink may search with.
func remoteSearchesAny(p provider.Provider) bool {
	return p.IsRemoteAgentLink() && slices.ContainsFunc(p.Available(), func(m catalog.Model) bool { return m.WebSearch == searchNative })
}

// searchableModel is a model of p that agentlink may search with: a Gemini
// on a Google sign-in, one a Remote agentlink searches natively for, any
// other provider's.
func searchableModel(p provider.Provider, model string) bool {
	switch {
	case googleAccount(p):
		return searchModel(model)
	case p.IsRemoteAgentLink():
		return remoteSearch(p, model) == searchNative
	}
	return true
}

// webSearchOf is what this agentlink's list tells another agentlink of how it
// searches for e (remoteSearch): agentlink is whether it has a searcher.
func webSearchOf(e provider.Entry, agentlink bool) string {
	p := e.Provider
	if e.Group == "" {
		if p.IsRemoteAgentLink() && remoteSearch(p, e.Model) == searchNative || codeAssistSearches(p, provider.CodeAssist, e.Model) {
			return searchNative
		}
		for _, proto := range p.Speaks() {
			if searchesItself(p, proto) || modelSearches(p, proto, e.Model) {
				return searchNative
			}
		}
	}
	if agentlink || searchesOwn(p) {
		return searchAgentLink
	}
	return ""
}

// markSearching marks a request to a remote agentlink made for a search.
func markSearching(ctx context.Context, req *http.Request) {
	if searching(ctx) {
		req.Header.Set(SearchingHeader, "1")
	}
}

// searchingFrom is whether a request is a search another agentlink made.
func searchingFrom(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("User-Agent"), "agentlink/") && r.Header.Get(SearchingHeader) != ""
}

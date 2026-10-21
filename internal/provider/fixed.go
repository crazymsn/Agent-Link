package provider

import (
	"errors"
	"slices"
	"strings"
)

const (
	// AgentLinkProviderID is the stable storage and routing id for the built-in
	// API distribution provider.
	AgentLinkProviderID   = "api-distribution"
	AgentLinkProviderName = "API 分发站"
	AgentLinkProviderURL  = "https://api.meding.site/v1"
	AgentLinkProviderIcon = "deepcloud"
)

var errFixedProvider = errors.New("the API 分发站 provider is built in and cannot be changed or removed")

var blockedAgentLinkModels = map[string]struct{}{
	"baai/bge-large-en-v1.5":  {},
	"baai/bge-large-zh-v1.5":  {},
	"baai/bge-m3":             {},
	"baai/bge-reranker-v2-m3": {},
	"chirp-v3-5":              {},
}

func blockedAgentLinkModel(id string) bool {
	_, ok := blockedAgentLinkModels[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

// AgentLinkProvider returns the canonical provider metadata. User-entered key
// material and model picks are merged from providers.json by allProviders.
func AgentLinkProvider() Provider {
	return Provider{
		ID:               AgentLinkProviderID,
		Name:             AgentLinkProviderName,
		Icon:             AgentLinkProviderIcon,
		Chat:             AgentLinkProviderURL,
		Responses:        AgentLinkProviderURL,
		Anthropic:        strings.TrimSuffix(AgentLinkProviderURL, "/v1"),
		Gemini:           AgentLinkProviderURL,
		EnabledProtocols: slices.Clone(allProtocols),
	}
}

// IsFixedProvider reports whether id is the protected built-in provider.
func IsFixedProvider(id string) bool { return id == AgentLinkProviderID }

func fixedProviderError() error { return errFixedProvider }

func mergeAgentLinkProvider(saved Provider) Provider {
	p := AgentLinkProvider()
	// The fixed provider has editable credentials, model picks, and an on/off
	// switch. Keep every endpoint and request setting canonical so an editor
	// payload cannot redirect requests or alter how its key is sent.
	p.Key = saved.Key
	p.KeyName = saved.KeyName
	p.Keys = slices.Clone(saved.Keys)
	p.KeyProtocol = saved.KeyProtocol
	p.KeyWeight = saved.KeyWeight
	p.EnabledProtocols = slices.Clone(allProtocols)
	p.PickedFrom = filterAgentLinkModels(saved.PickedFrom)
	p.Models = filterAgentLinkModels(saved.Models)
	p.NoModels = saved.NoModels
	p.Off = saved.Off
	return p
}

func filterAgentLinkModels(models []string) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if !blockedAgentLinkModel(model) && !slices.Contains(out, model) {
			out = append(out, model)
		}
	}
	return out
}

// MigrateFixedProvider removes user-defined custom providers and normalizes the
// built-in provider. Presets and subscription/plugin records remain available.
func MigrateFixedProvider() error {
	f, err := read()
	if err != nil {
		return err
	}
	changed := false
	providers := make([]Provider, 0, len(f.Providers)+1)
	for _, p := range f.Providers {
		p = normalize(p)
		if IsFixedProvider(p.ID) {
			providers = append(providers, mergeAgentLinkProvider(p))
			changed = true
			continue
		}
		if hasEndpoint(p) && p.Preset == "" && !subscriptionID(p.ID) {
			changed = true
			continue
		}
		providers = append(providers, p)
	}
	if !slices.ContainsFunc(providers, func(p Provider) bool { return p.ID == AgentLinkProviderID }) {
		providers = append([]Provider{AgentLinkProvider()}, providers...)
		changed = true
	}
	for i, id := range f.Order {
		if id == AgentLinkProviderID {
			f.Order = append(f.Order[:i], f.Order[i+1:]...)
			changed = true
			break
		}
	}
	f.Providers = providers
	if !changed {
		return nil
	}
	return store(f)
}

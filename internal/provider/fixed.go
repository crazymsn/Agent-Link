package provider

import (
	"errors"
	"slices"
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

// AgentLinkProvider returns the canonical provider metadata. User-entered key
// material and model picks are merged from providers.json by allProviders.
func AgentLinkProvider() Provider {
	return Provider{
		ID:   AgentLinkProviderID,
		Name: AgentLinkProviderName,
		Icon: AgentLinkProviderIcon,
		Chat: AgentLinkProviderURL,
	}
}

// IsFixedProvider reports whether id is the protected built-in provider.
func IsFixedProvider(id string) bool { return id == AgentLinkProviderID }

func fixedProviderError() error { return errFixedProvider }

func mergeAgentLinkProvider(saved Provider) Provider {
	p := AgentLinkProvider()
	// The fixed provider has one editable credential, one model selection, and
	// an on/off switch. Keep every endpoint, proxy and request setting canonical
	// so an editor payload cannot redirect requests or alter how its key is sent.
	p.Key = saved.Key
	p.Models = slices.Clone(saved.Models)
	p.NoModels = saved.NoModels
	p.Off = saved.Off
	return p
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

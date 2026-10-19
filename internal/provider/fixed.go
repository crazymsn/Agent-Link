package provider

import (
	"errors"
	"slices"
)

const (
	// AgentLinkProviderID is the stable storage and routing id for the built-in
	// API distribution provider.
	AgentLinkProviderID = "api-distribution"
	AgentLinkProviderName = "API 分发站"
	AgentLinkProviderURL = "https://api.meding.site/v1"
	AgentLinkProviderIcon = "deepcloud"
)

var errFixedProvider = errors.New("the API 分发站 provider is built in and cannot be changed or removed")

// AgentLinkProvider returns the canonical provider metadata. User-entered key
// material and model picks are merged from providers.json by allProviders.
func AgentLinkProvider() Provider {
	return Provider{
		ID: AgentLinkProviderID,
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
	p.Key = saved.Key
	p.Keys = slices.Clone(saved.Keys)
	p.KeyName = saved.KeyName
	p.KeyProtocol = saved.KeyProtocol
	p.KeyWeight = saved.KeyWeight
	p.Models = slices.Clone(saved.Models)
	p.PickedFrom = slices.Clone(saved.PickedFrom)
	p.NoModels = saved.NoModels
	p.Unlisted = saved.Unlisted
	p.Off = saved.Off
	p.Proxy = saved.Proxy
	p.AccountModels = saved.AccountModels
	p.AccountCaps = saved.AccountCaps
	p.AccountWindowCaps = saved.AccountWindowCaps
	p.AccountConcurrency = saved.AccountConcurrency
	p.QueueLimit = saved.QueueLimit
	p.QueueWait = saved.QueueWait
	p.MaxRPM = saved.MaxRPM
	p.QuotaRest = saved.QuotaRest
	p.QuotaResetAt = saved.QuotaResetAt
	p.RestCap = saved.RestCap
	p.Routing = saved.Routing
	p.Sink = saved.Sink
	p.Affinity = saved.Affinity
	p.Fallback = slices.Clone(saved.Fallback)
	p.Contexts = saved.Contexts
	p.Family = saved.Family
	p.MaxConcurrency = saved.MaxConcurrency
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

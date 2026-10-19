package plugin

import (
	"encoding/json"
)

// withPluginMetadata keeps the current agentlink manifest name while accepting
// the community plugins' established magpie metadata name. Agent Link fields
// win when a package includes both names, so a package cannot override an
// explicit Agent Link setting by adding the compatibility field.
func withPluginMetadata(b []byte) []byte {
	var pkg map[string]json.RawMessage
	if json.Unmarshal(b, &pkg) != nil {
		return b
	}
	merged := map[string]json.RawMessage{}
	for _, key := range []string{"magpie", "agentlink"} {
		var fields map[string]json.RawMessage
		if raw, ok := pkg[key]; !ok || json.Unmarshal(raw, &fields) != nil {
			continue
		}
		for name, value := range fields {
			merged[name] = value
		}
	}
	if len(merged) == 0 {
		return b
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return b
	}
	pkg["agentlink"] = raw
	out, err := json.Marshal(pkg)
	if err != nil {
		return b
	}
	return out
}

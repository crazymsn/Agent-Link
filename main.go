// agentlink — one place to pick every agent's model. The commands live in
// internal/cli; this is only where the binary starts.
package main

import (
	"github.com/crazymsn/Agent-Link/internal/cli"
	// the built-in plugins, registered with the runtime (internal/pluginrt)
	_ "github.com/crazymsn/Agent-Link/plugins/all"
)

// version is set at build time: -ldflags "-X main.version=…" (Makefile,
// Dockerfile, agentlink-releases), so it stays in package main.
var version = "dev"

func main() { cli.Main(version) }

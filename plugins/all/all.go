// Package all registers every built-in plugin. Only agentlink's main imports
// it: core never imports a plugin, and a plugin never imports another.
package all

import (
	_ "github.com/crazymsn/Agent-Link/plugins/budget"
	_ "github.com/crazymsn/Agent-Link/plugins/gateway"
	_ "github.com/crazymsn/Agent-Link/plugins/groups"
	_ "github.com/crazymsn/Agent-Link/plugins/library"
	_ "github.com/crazymsn/Agent-Link/plugins/middleware"
	_ "github.com/crazymsn/Agent-Link/plugins/otel"
	_ "github.com/crazymsn/Agent-Link/plugins/redact"
	_ "github.com/crazymsn/Agent-Link/plugins/rotation"
	_ "github.com/crazymsn/Agent-Link/plugins/routing"
	_ "github.com/crazymsn/Agent-Link/plugins/search"
	_ "github.com/crazymsn/Agent-Link/plugins/sessions"
	_ "github.com/crazymsn/Agent-Link/plugins/tools"
	_ "github.com/crazymsn/Agent-Link/plugins/usage"
	_ "github.com/crazymsn/Agent-Link/plugins/warmups"
)

package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/crazymsn/Agent-Link/internal/access"
	"github.com/crazymsn/Agent-Link/internal/agent"
	"github.com/crazymsn/Agent-Link/internal/gateway"
	"github.com/crazymsn/Agent-Link/internal/mcpauth"
)

// A remote server that asks for an OAuth sign-in is signed in to once, in
// agentlink, and every agent given it reaches it through the gateway's
// /mcp/<name>, which adds agentlink's token (#615): the agents are given that
// address in place of the server's. A server agentlink isn't signed in to is
// given as it is, and the agent signs in itself as before.

func init() {
	mcpauth.Changed = func(string) { _, _ = Sync() }
	// an agent beyond loopback is given a relayed server with the LAN key
	// (through), which is the one there is no longer
	access.LANChanged = func() {
		if relaysAny() {
			_, _ = Sync()
		}
	}
}

// relaysAny says whether any of the library's servers is given through the
// gateway: one agentlink is signed in to.
func relaysAny() bool {
	mu.Lock()
	l, err := load()
	mu.Unlock()
	if err != nil {
		return false
	}
	return slices.ContainsFunc(l.MCP, func(s *Server) bool { return s.Transport == "http" && mcpauth.SignedIn(s.Name, s.URL) })
}

// through is the server as an agent that reaches the gateway at base is
// given it. Only a streamable HTTP server is relayed: an SSE one's stream
// names the address to post to, which would have to be rewritten.
func through(s *Server, base string) *Server {
	if s.Transport != "http" || !mcpauth.SignedIn(s.Name, s.URL) {
		return s
	}
	c := *s
	c.URL = strings.TrimRight(base, "/") + "/mcp/" + url.PathEscape(s.Name)
	c.Headers = nil
	for k, v := range s.Headers {
		// agentlink's token goes in its place
		if strings.EqualFold(k, "Authorization") {
			continue
		}
		if c.Headers == nil {
			c.Headers = map[string]string{}
		}
		c.Headers[k] = v
	}
	if k := agent.KeyAt(base); k != gateway.Token {
		// from another machine (a WSL distro under NAT) the gateway refuses
		// a call without an enabled key, with a 401 the agent would take
		// for the server asking it to sign in; the relay drops this
		// Authorization for agentlink's token
		if c.Headers == nil {
			c.Headers = map[string]string{}
		}
		c.Headers["Authorization"] = "Bearer " + k
	}
	return &c
}

// gatewayOf is the gateway's address as the target's agent reaches it.
func gatewayOf(t *Target) string {
	if t.Agent != nil && t.Agent.Gateway != nil {
		return t.Agent.Gateway()
	}
	return gateway.URL()
}

var errNotHTTP = errors.New("agentlink signs in only to a server reached over streamable HTTP")

// SignInServer starts signing agentlink in to the library's server called
// name: the State's URL is the page to open in a browser.
func SignInServer(ctx context.Context, name string) (mcpauth.State, error) {
	mu.Lock()
	l, err := load()
	mu.Unlock()
	if err != nil {
		return mcpauth.State{}, err
	}
	s := l.server(name)
	if s == nil {
		return mcpauth.State{}, fmt.Errorf("no server called %s", name)
	}
	if s.Transport != "http" {
		return mcpauth.State{}, errNotHTTP
	}
	var app *mcpauth.App
	if s.OAuth != nil {
		app = &mcpauth.App{ClientID: s.OAuth.ClientID, ClientSecret: s.OAuth.ClientSecret, CallbackPort: s.OAuth.CallbackPort}
	}
	return mcpauth.StartWith(ctx, s.Name, s.URL, app)
}

// SignOutServer forgets agentlink's sign-in to the server called name, and
// gives the agents the server's own address again.
func SignOutServer(name string) error { return mcpauth.SignOut(name) }

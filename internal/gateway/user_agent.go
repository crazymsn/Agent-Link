package gateway

import (
	"context"
	"net/http"
	"strings"

	"github.com/crazymsn/Agent-Link/internal/provider"
)

// agentlinkUA is the User-Agent agentlink's requests go to a vendor with.
func agentlinkUA() string { return "agentlink/" + Version }

// userAgentFor is the User-Agent a request to p goes with, ua being the
// one the request came to agentlink with: the agent's own, as it sent it,
// where p is set to pass it on (provider.PassesUserAgent, #1851), and
// agentlink's where it isn't, where the agent sent none, and on agentlink's own
// requests — its classifier's, vision's, search's, the image tool's
// (agentlinksOwn) — which no agent sent. A User-Agent the user set among the
// provider's headers is put on after, when the request is signed, and
// stands over either.
func userAgentFor(p provider.Provider, ua string) string {
	if ua != "" && p.PassesUserAgent() && !agentlinksOwn(ua) {
		return ua
	}
	return agentlinkUA()
}

// agentlinksOwn is a User-Agent agentlink's own requests carry: another agentlink's
// (agentlink/…), the classifier's, vision's and search's (agentlink-router/1,
// agentlink-vision/1, agentlink-search/1), and the image tool's, which names the
// agent it draws for before its own (claude-code/2.1.0 agentlink-image/1)
// from what the agent told it, not from a User-Agent the agent sent.
func agentlinksOwn(ua string) bool {
	return strings.HasPrefix(ua, "agentlink/") || strings.HasPrefix(ua, "agentlink-") || strings.HasSuffix(ua, " "+DrawAgent)
}

// callerUA is the User-Agent the request ctx is for came with (withCaller),
// "" when there is none.
func callerUA(ctx context.Context) string {
	c, _ := ctx.Value(callerCtx{}).(caller)
	return c.ua
}

// stampUA puts on req, a request to p that isn't agentlink's own forwarded
// one (an embedding, a drawing, a video), the User-Agent forwardOnce's
// carry: agentlink's, or the agent's where p passes it on. A signed-in
// account's is left to its signing, as before, and another agentlink's is
// set where it is asked.
func stampUA(ctx context.Context, p provider.Provider, h http.Header) {
	if p.Account != nil || p.IsRemoteAgentLink() {
		return
	}
	h.Set("User-Agent", userAgentFor(p, callerUA(ctx)))
}

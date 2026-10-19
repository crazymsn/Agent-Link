package gui

import (
	"net"
	"os"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/fx"
	"github.com/crazymsn/Agent-Link/internal/testenv"
)

// TestMain keeps the rate off the network: a state read with no rate cached
// asks for one behind it (#541), which would otherwise reach the real
// endpoint and land in whatever test runs next. And the tests run in a
// home of their own, never the user's real files.
func TestMain(m *testing.M) {
	// started by TestGatewayFixQuitsOnlyAAgentLink as an older agentlink
	if addr := os.Getenv("AGENT_LINK_TEST_FAKE_GATEWAY"); addr != "" {
		fakeOlderGateway(addr)
		return
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		addr := l.Addr().String()
		l.Close() // refused at once from here on
		fx.SetRateURL("http://" + addr + "/")
	}
	os.Exit(testenv.Run(m))
}

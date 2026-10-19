package gui

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crazymsn/Agent-Link/internal/agent"
	"github.com/crazymsn/Agent-Link/internal/catalog"
	"github.com/crazymsn/Agent-Link/internal/gateway"
	"github.com/crazymsn/Agent-Link/internal/provider"
	"github.com/crazymsn/Agent-Link/internal/update"
)

// served is the gateway this process serves, nil while another agentlink
// has it.
var served atomic.Pointer[gateway.Server]

// backendCtx ends the gateway this process serves (stopServing).
var backendCtx, endBackend = context.WithCancel(context.Background())

// serving is the gateway this process serves, until it has finished.
var serving sync.WaitGroup

// stopServing stops this process's gateway taking requests and returns
// once those in flight have finished.
func stopServing() {
	endBackend()
	serving.Wait()
}

// startBackend starts what serves the page and the agents: the gateway,
// unless another agentlink has it (then that one serves and this one only
// shows its status, and gw is nil — until that one is gone: a agentlink left
// running from before an update, a agentlink serve in a terminal), and the
// model lists kept warm.
func startBackend() (gw *gateway.Server) {
	if err := provider.MigrateFixedProvider(); err != nil {
		log.Println("provider migration:", err)
	}
	gateway.Window = true // the routing this process serves is shown on its page
	gw = serveGateway()
	go watchGateway(backendCtx, gatewayWatch)
	// Codex's background app-server, restarted when it has the list from
	// before a change and no codex session is on it (luci)
	go agent.KeepCodexDaemonCurrent(backendCtx)
	// Model lists are fetched, never compiled in: whatever the agents can see
	// comes from the models.dev catalog plus each vendor's own /models answer.
	// Keep both halves warm without making the user click anything.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if catalog.Stale() {
			if err := catalog.Sync(ctx); err != nil {
				log.Println("catalog:", err)
			}
		}
		cancel()
		// A signed-in agent's list exists only at the vendor; fill it in the
		// first time so the picker never shows a stale snapshot.
		provider.FetchNew(20 * time.Second)
		// lists an older agentlink wrote into agents' files, without what
		// it has learnt since (context windows, providers added)
		agent.SyncCatalog()
		// and fetched again each day it stays open, for new models' prices
		catalog.KeepFresh()
	}()
	return gw
}

// gatewayWatch is how often a agentlink without the gateway looks for it gone.
const gatewayWatch = 15 * time.Second

// watchGateway takes the gateway up once the agentlink that had it is gone,
// looking every so often until ctx ends: a watch that outlived it would
// start a gateway after this one has stopped serving.
func watchGateway(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		gatewayMu.Lock()
		if ctx.Err() == nil && served.Load() == nil && !gateway.Running() {
			serveGatewayLocked()
		}
		gatewayMu.Unlock()
	}
}

// takenSaid: the log has said the gateway's port is taken by one that
// doesn't answer, said once until it is free again.
var takenSaid atomic.Bool

// gatewayMu keeps one start of the gateway here at a time: the watch's,
// a restart's, a take-over's (takeover.go).
var gatewayMu sync.Mutex

// servedRun ends the gateway this process serves, and is closed once it
// has, for a restart; under gatewayMu.
var servedRun struct {
	stop context.CancelFunc
	done chan struct{}
}

// serveGateway starts the gateway here when no agentlink has it: the one
// started, or nil.
func serveGateway() *gateway.Server {
	gatewayMu.Lock()
	defer gatewayMu.Unlock()
	return serveGatewayLocked()
}

func serveGatewayLocked() *gateway.Server {
	// handing over, the one there is this one's predecessor, which lets go
	// once this one listens beside it
	if !gateway.Handover {
		if o := gateway.ServedBy(); o.Running {
			if update.Newer(gateway.Version, o.Version) {
				log.Printf("gateway: agentlink %s serves %s, older than this one (%s); agents' requests go through it until it quits", o.Version, gateway.URL(), gateway.Version)
			}
			return nil
		}
		// a agentlink too busy to answer the question in time (a small
		// server, one starting), or another program, has the port: a
		// second listen only fails (K.Liu on Discord: agentlink web beside
		// the service logged "bind: address already in use")
		if gateway.Taken() {
			if !takenSaid.Swap(true) {
				log.Printf("gateway: %s is taken but doesn't answer as agentlink yet; this agentlink serves the gateway once it is free", gateway.URL())
			}
			return nil
		}
	}
	takenSaid.Store(false)
	gw := gateway.New()
	served.Store(gw)
	serving.Add(1)
	ctx, stop := context.WithCancel(backendCtx)
	done := make(chan struct{})
	servedRun.stop, servedRun.done = stop, done
	go func() {
		defer serving.Done()
		defer close(done)
		defer stop()
		if err := gw.ListenAndServe(ctx); err != nil {
			log.Println("gateway:", err)
			served.CompareAndSwap(gw, nil) // another took the port first
		}
	}()
	return gw
}

package node

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/status"
	"github.com/fedishare/fedishare/internal/tunnel"
)

func (n *Node) PublicHandler() http.Handler {
	n.mu.Lock()
	h := n.public
	n.mu.Unlock()
	if h != nil {
		return h
	}
	return httpserver.PublicHandler(n)
}

func (n *Node) ensureTunnelLocked() {
	if n.runCtx == nil {
		return
	}
	if !n.cfg.Configured() || strings.TrimSpace(n.cfg.GatewayURL) == "" {
		if n.tunnelCancel != nil {
			n.tunnelCancel()
			n.tunnelCancel = nil
		}
		n.tunnelStarted = false
		n.gatewayUp = false
		n.connecting = false
		return
	}
	if n.tunnelStarted {
		return
	}
	n.tunnelStarted = true
	n.tunnelCtx, n.tunnelCancel = context.WithCancel(n.runCtx)
	go n.runTunnel(n.tunnelCtx)
}

func (n *Node) restartTunnelLocked() {
	if n.tunnelCancel != nil {
		n.tunnelCancel()
		n.tunnelCancel = nil
	}
	n.tunnelStarted = false
	n.gatewayUp = false
	n.connecting = false
	n.ensureTunnelLocked()
}

func (n *Node) runTunnel(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 60 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		n.setTunnelFlags(false, true)
		start := time.Now()
		err := n.connectOnce(ctx)
		n.setTunnelFlags(false, false)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			n.log.Warn("gateway tunnel", "err", err)
		}
		if time.Since(start) > 30*time.Second {
			backoff = time.Second
		}
		jitter := time.Duration(rand.Int64N(int64(backoff/5) + 1))
		timer := time.NewTimer(backoff + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (n *Node) setTunnelFlags(up, connecting bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.gatewayUp = up
	n.connecting = connecting
	_ = n.refreshLocked(false)
}

func (n *Node) connectOnce(ctx context.Context) error {
	n.mu.Lock()
	gw := n.cfg.GatewayURL
	user := n.cfg.Username
	display := n.cfg.DisplayName
	if display == "" {
		display = user
	}
	nodeID := n.nodeID
	n.mu.Unlock()

	priv, err := n.keys.PrivateKey()
	if err != nil {
		return err
	}
	pem, err := n.keys.PublicKeyPEM()
	if err != nil {
		return err
	}
	client := &tunnel.Client{
		GatewayURL:   gw,
		Username:     user,
		PublicKeyPEM: pem,
		NodeID:       nodeID,
		DisplayName:  display,
		Key:          priv,
		Log:          n.log,
	}
	if err := client.Connect(ctx); err != nil {
		_ = client.Close()
		return err
	}
	n.setTunnelFlags(true, false)
	err = client.Serve(ctx, n.PublicHandler())
	_ = client.Close()
	return err
}

func (n *Node) gatewayMessage(next *status.State, msg *string) {
	if !n.cfg.Configured() {
		return
	}
	if n.paused {
		*next = status.StatePaused
		*msg = "Sharing paused"
		return
	}
	if n.gatewayUp {
		*next = status.StateOnline
		*msg = "Gateway connected"
		return
	}
	if n.connecting {
		*next = status.StateConnecting
		*msg = "Connecting to gateway"
	}
}

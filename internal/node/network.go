package node

import (
	"context"

	"github.com/fedishare/fedishare/internal/network"
)

func (n *Node) FetchNetwork(ctx context.Context, query string) (network.Directory, error) {
	if n == nil || n.cfg == nil {
		return network.Directory{
			Type:   network.TypeDirectory,
			Actors: []network.Actor{},
			Hint:   "Finish setup before browsing the FediShare network.",
		}, nil
	}
	c := network.NewClient(n.allowPrivateFederation())
	return network.Resolve(ctx, c, query, n.cfg.GatewayURL)
}

func (h *Host) FetchNetwork(ctx context.Context, query string) (network.Directory, error) {
	if n := h.current(); n != nil {
		return n.FetchNetwork(ctx, query)
	}
	return network.Directory{
		Type:   network.TypeDirectory,
		Actors: []network.Actor{},
		Hint:   "Finish setup before browsing the FediShare network.",
	}, nil
}

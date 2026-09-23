//go:build !cgo

package tray

import (
	"context"
)

// Run waits until ctx is cancelled. The real tray requires CGO.
func Run(ctx context.Context, opts Options) {
	if opts.Log != nil {
		opts.Log.Info("system tray disabled (built without cgo)")
	}
	<-ctx.Done()
	if opts.OnQuit != nil {
		opts.OnQuit()
	}
}

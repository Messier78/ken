package server

import (
	"context"

	"golang.org/x/sync/errgroup"

	"ken/lib/rtmp"
)

var (
	g   *errgroup.Group
	ctx context.Context
)

func init() {
	g, ctx = errgroup.WithContext(context.Background())
}

func Start() error {
	g.Go(func() error {
		return rtmp.StartServer(ctx, "tcp", "localhost:1935", nil)
	})

	return g.Wait()
}

package server

import (
	"context"

	"golang.org/x/sync/errgroup"

	"ken/server/rtmp"
)

var (
	g   *errgroup.Group
	ctx context.Context
)

func init() {
	g, ctx = errgroup.WithContext(context.Background())
}

func Start() {
	g.Go(func() error {
		return rtmp.StartServer(ctx, "tcp", "localhost")
	})
}

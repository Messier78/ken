package server

import (
	"context"

	"golang.org/x/sync/errgroup"

	"ken/lib/http"
	"ken/lib/rtmp"
	"ken/service"
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
		return rtmp.StartServer("tcp", ":1935", service.GetHandler(ctx))
	})

	g.Go(func() error {
		return http.StartServer(":8080")
	})

	return g.Wait()
}

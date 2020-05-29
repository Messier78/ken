package server

import (
	"context"
	"fmt"

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
	h := service.GetHandler(ctx)
	g.Go(func() error {
		fmt.Println("--------- start rtmp server ----------")
		return rtmp.StartServer("tcp", ":1935", h)
	})

	g.Go(func() error {
		fmt.Println("---------- start http server -----------")
		return http.StartServer(":8080")
	})

	err := g.Wait()
	fmt.Println("----------- server stop ----------------")
	return err
}

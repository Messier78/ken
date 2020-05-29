// service
package service

import (
	"context"
	"sync"

	ks "ken/service/ken_service"
	"ken/types"
)

var (
	defaultHandler ServiceHandler
	once           sync.Once
)

type ServiceHandler interface {
	Ctx() context.Context
	OnPlay(cont *types.Content) context.Context
	OnPublish(cont *types.Content) (context.Context, int)
	OnPull(ctx context.Context)
	OnPush(ctx context.Context)
	OnMeta(ctx context.Context)
	OnRecord(ctx context.Context)
	OnSlice(ctx context.Context)
	OnMetaType(ctx context.Context)
	OnMetaWaitVideo(ctx context.Context)
	CloseStream(domain, app, name string)
}

func GetHandler(ctx context.Context) ServiceHandler {
	once.Do(func() {
		defaultHandler = ks.ServiceHandler(ctx)
	})

	return defaultHandler
}

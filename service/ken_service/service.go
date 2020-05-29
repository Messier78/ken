package ken_service

import (
	"context"
	"sync"

	"ken/types"
)

var (
	once sync.Once
	ks   *kenService
)

type appMap map[string]streamMap
type streamMap map[string]*stream
type HandlerFunc func(ctx context.Context)
type HandlersChain []HandlerFunc

type stream struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type kenService struct {
	ctx     context.Context
	domains map[string]appMap

	play    HandlersChain
	publish HandlersChain

	mu sync.RWMutex
}

func ServiceHandler(ctx context.Context) *kenService {
	once.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		ks = &kenService{
			ctx:     context.Background(),
			domains: make(map[string]appMap),
		}
		ks.useOnPlay(
			blockOnPlay,
			pullOnPlay,
		)
		ks.useOnPublish(
			blockOnPublish,
			pushOnPublish,
		)
	})
	return ks
}

func (ks *kenService) Ctx() context.Context {
	return ks.ctx
}

func (ks *kenService) OnPlay(cont *types.Content) context.Context {
	ks.mu.Lock()
	am, ok := ks.domains[cont.Domain]
	if !ok {
		am = make(appMap)
		ks.domains[cont.Domain] = am
	}
	sm, ok := am[cont.App]
	if !ok {
		sm = make(streamMap)
		am[cont.App] = sm
	}
	s, ok := sm[cont.Name]
	if !ok {
		s = &stream{}
		s.ctx, s.cancel = context.WithCancel(ks.ctx)
		sm[cont.Name] = s
	}
	ks.mu.Unlock()

	ctx := types.NewContext(s.ctx, cont)
	// TODO: plugin/middleware
	return ctx
}

func (ks *kenService) OnPublish(cont *types.Content) (context.Context, int) {
	ks.mu.Lock()
	am, ok := ks.domains[cont.Domain]
	if !ok {
		am = make(appMap)
		ks.domains[cont.Domain] = am
	}
	sm, ok := am[cont.App]
	if !ok {
		sm = make(streamMap)
		am[cont.App] = sm
	}
	s, ok := sm[cont.Name]
	if !ok {
		s = &stream{}
		s.ctx, s.cancel = context.WithCancel(ks.ctx)
		sm[cont.Name] = s
	}
	ks.mu.Unlock()
	return types.NewContext(s.ctx, cont), 200
}

func (ks *kenService) OnPull(ctx context.Context) {

}

func (ks *kenService) OnPush(ctx context.Context) {

}

func (ks *kenService) OnMeta(ctx context.Context) {

}

func (ks *kenService) OnRecord(ctx context.Context) {

}

func (ks *kenService) OnSlice(ctx context.Context) {

}

func (ks *kenService) OnMetaType(ctx context.Context) {

}

func (ks *kenService) OnMetaWaitVideo(ctx context.Context) {

}

func (ks *kenService) CloseStream(domain, app, name string) {
	if domain == "" {
		return
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if am, ok := ks.domains[domain]; ok {
		if app == "" {
			for _, sm := range am {
				for _, s := range sm {
					s.cancel()
				}
			}
		} else {
			if sm, ok := am[app]; ok {
				if name == "" {
					for _, s := range sm {
						s.cancel()
					}
				} else {
					if s, ok := sm[name]; ok {
						s.cancel()
					}
				}
			}
		}
	}
}

func (ks *kenService) useOnPlay(middleware ...HandlerFunc) {
	ks.play = append(ks.play, middleware...)
}

func (ks *kenService) useOnPublish(middleware ...HandlerFunc) {
	ks.publish = append(ks.publish, middleware...)
}

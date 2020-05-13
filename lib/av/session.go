package av

import (
	"sync"
)

var (
	sessions    sync.Map
	sessionPool *sync.Pool
)

func init() {
	sessionPool = &sync.Pool{
		New: func() interface{} {
			return newStream()
		},
	}
}

type Stream struct {
	*Cache
	key string
}

func newStream() *Stream {
	return &Stream{
		Cache: NewCache(),
	}
}

func AttachToStream(key string) *Stream {
	ss := sessionPool.Get()
	v, ok := sessions.LoadOrStore(key, sessionPool.Get())
	if ok {
		sessionPool.Put(ss)
	} else {
		v.(*Stream).SetID(key)
	}
	s := v.(*Stream)
	s.key = key
	return s
}

func (s *Stream) Reset() {
}

func (s *Stream) Idx() int64 {
	return s.Cache.Idx
}

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
			return newSession()
		},
	}
}

type Session struct {
	key string

	gop *Cache
}

func newSession() *Session {
	return &Session{
		gop: NewCache(),
	}
}

func AttachToSession(key string) *Session {
	ss := sessionPool.Get()
	v, ok := sessions.LoadOrStore(key, sessionPool.Get())
	if ok {
		sessionPool.Put(ss)
	} else {
		v.(*Session).gop.SetID(key)
	}
	s := v.(*Session)
	s.key = key
	return s
}

func (s *Session) Reset() {
	// TODO:
}

func (s *Session) Idx() int64 {
	return s.gop.Idx
}

func (s *Session) NewWriter(genID int) (PacketWriter, error) {
	return s.gop.NewPacketWriter(genID)
}

func (s *Session) NewReader() PacketReader {
	return s.gop.NewPacketReader()
}

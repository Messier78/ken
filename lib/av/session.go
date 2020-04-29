package av

import (
	"container/ring"
	"sync"

	"github.com/scythefly/orb"
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

type Publisher interface {
	GenID() int
	GetStartPos() (avc, aac *Packet, r *ring.Ring, cond *sync.Cond)
	Idx() int64
}

type Player interface {
}

type Session struct {
	key           string
	currentStream Publisher
	publishers    orb.Set
	players       orb.Set
}

func newSession() *Session {
	return &Session{
		publishers: orb.NewSet(),
		players:    orb.NewSet(),
	}
}

func AttachToSession(key string) *Session {
	v, _ := sessions.LoadOrStore(key, newSession())
	s := v.(*Session)
	s.key = key
	return s
}

func (s *Session) Reset() {
	// TODO:
}

func (s *Session) GetStartPos() (avc, aac *Packet, r *ring.Ring, cond *sync.Cond) {
	if s.currentStream != nil {
		return s.currentStream.GetStartPos()
	}
	return nil, nil, nil, nil
}

func (s *Session) HandlePublishStream(p Publisher) {
	s.publishers.Add(p)
	if s.currentStream == nil {
		s.currentStream = p
	}

	// TODO: overflow
	if p.GenID() > s.currentStream.GenID() {
		s.currentStream = p
	}
}

func (s *Session) HandlePlayStream(p Player) {
	s.players.Add(p)
}

func (s *Session) Idx() int64 {
	if s.currentStream != nil {
		return s.currentStream.Idx()
	}
	return -1
}

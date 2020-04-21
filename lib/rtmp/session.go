package rtmp

import (
	"sync"

	"github.com/scythefly/orb"

	gop2 "ken/lib/gop"
)

var (
	sessions sync.Map
)

type Session struct {
	key           string
	currentStream *inboundStream
	iss           orb.Set
	oss           orb.Set
	gop           *gop2.Cache
}

func newSession() *Session {
	return &Session{
		iss: orb.NewSet(),
		oss: orb.NewSet(),
		gop: gop2.NewGopCache(),
	}
}

func appendPublishConn(key string, stream *inboundStream) InboundStreamHandler {
	v, _ := sessions.LoadOrStore(key, newSession())
	s := v.(*Session)
	s.key = key
	return s.handlePublishStream(stream)
}

func appendPlayConn(key string, stream *inboundStream) InboundStreamHandler {
	v, _ := sessions.LoadOrStore(key, newSession)
	s := v.(*Session)
	s.key = key
	return s.handlePlayStream(stream)
}

func removeConn(key string, stream *inboundStream) {
	if v, ok := sessions.Load(key); ok {
		s := v.(*Session)
		s.deleteStream(stream)
	}
}

func (s *Session) handlePublishStream(stream *inboundStream) InboundStreamHandler {
	s.iss.Add(stream)
	if s.currentStream == nil {
		s.currentStream = stream
		return s
	}

	//
	if stream.genID > s.currentStream.genID {
		s.currentStream.Attach(nooo)
		s.currentStream = stream
		return s
	}
	return nooo
}

func (s *Session) handlePlayStream(stream *inboundStream) InboundStreamHandler {
	logger.Debugf(">>> play session attached...")
	s.oss.Add(stream)
	return s
}

func (s *Session) deleteStream(stream *inboundStream) {
	if s.oss.Contains(stream) {
		s.oss.Remove(stream)
		return
	}
	// TODO: iss
}

func (s *Session) OnPlayStart(stream InboundStream) {
	logger.Debugf(">>> play start, key: %s", s.key)
}

func (s *Session) OnPublishStart(stream InboundStream) {
	logger.Debugf(">>> publish start, key: %s", s.key)
}

func (s *Session) OnReceived(msg *Message) bool {
	// logger.Debugf(">>>> Session receive mssage, type=%d", msg.Type)
	var isKeyFrame bool = false
	if msg.Type == AUDIO_TYPE || msg.Type == VIDEO_TYPE {
		// TODO: codec and key frame
	}
	s.gop.WritePacket(msg.Buf, isKeyFrame)
	s.gop.Broadcast()
	return true
}

func (s *Session) OnReceiveAudio(stream InboundStream, on bool) {

}
func (s *Session) OnReceiveVideo(stream InboundStream, on bool) {

}

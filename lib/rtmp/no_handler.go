package rtmp

// noHandler
// used for stream coverage
var nooo = &noHandler{}

type noHandler struct {
}

func (s *noHandler) OnPlayStart(stream InboundStream) {
}

func (s *noHandler) OnPublishStart(stream InboundStream) {
}

func (s *noHandler) OnReceived(msg *Message) bool {
	return true
}

func (s *noHandler) OnReceiveAudio(stream InboundStream, on bool) {
}

func (s *noHandler) OnReceiveVideo(stream InboundStream, on bool) {
}

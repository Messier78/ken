package rtmp

import (
	"bufio"
	"context"
	"net"
)

type inboundManager struct {
}

var defaultInManager = &inboundManager{}

func (m *inboundManager) NewInboundConn(ctx context.Context, c net.Conn, r *bufio.Reader, w *bufio.Writer,
	authHandler InboundAuthHandler, maxChannelNumber int) (InboundConn, error) {
	return NewInboundConn(ctx, c, r, w, authHandler, maxChannelNumber)
}

func (m *inboundManager) OnConnectAuth(iconn InboundConn, cmd *Command) bool {
	logger.Debugf("OnConnectAuth, command: %s", cmd.Name)
	iconn.Attach(m)
	return true
}

func (m *inboundManager) OnStatus(iconn InboundConn) {
	status, _ := iconn.Status()
	logger.Debugf("OnStatus: %d", status)
}

func (m *inboundManager) OnStreamCreated(iconn InboundConn, stream InboundStream) {
	logger.Debugf("OnStreamCreated, stream id: %d", stream.ID())
}

func (m *inboundManager) OnStreamClosed(iconn InboundConn, stream InboundStream) {
	logger.Debugf("OnStreamClosed, stream id: %d", stream.ID())
}

func (m *inboundManager) OnReceived(conn Conn, msg *Message) {
	logger.Debugf(">> OnReceived, type: %d", msg.Type)
}

func (m *inboundManager) OnReceivedRtmpCommand(conn Conn, cmd *Command) {
	logger.Debugf(">> OnReceivedRtmpCommand, cmd: %s", cmd.Name)
}

func (m *inboundManager) OnClosed(conn Conn) {
	logger.Debugf(">> OnClosed")
}

func (m *inboundManager) OnPlayStart(stream InboundStream) {
	logger.Debugf(">> OnPlayStart")
}

func (m *inboundManager) OnPublishStart(stream InboundStream) {
	logger.Debugf(">> OnPublishStart")
}

func (m *inboundManager) OnReceiveAudio(stream InboundStream, on bool) {
	logger.Debugf("OnReceiveAudio")
}

func (m *inboundManager) OnReceiveVideo(stream InboundStream, on bool) {
	logger.Debugf("OnReceiveVideo")
}

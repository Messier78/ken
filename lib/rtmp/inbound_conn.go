package rtmp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"

	"ken/lib/amf"
	"ken/lib/av"
)

const (
	INBOUND_CONN_STATUS_CLOSE            = uint(0)
	INBOUND_CONN_STATUS_CONNECT_OK       = uint(1)
	INBOUND_CONN_STATUS_CREATE_STREAM_OK = uint(2)
)

type InboundAuthHandler interface {
	OnConnectAuth(ibConn InboundConn, connectReq *Command) bool
}

type InboundConnHandler interface {
	ConnHandler
	OnStatus(iConn InboundConn)
	OnStreamCreated(iConn InboundConn, stream InboundStream)
	OnStreamClosed(iConn InboundConn, stream InboundStream)
}

type InboundConn interface {
	Close()
	Status() (uint, error)
	Send(msg *Message) error
	Call(params ...interface{}) (err error)
	Conn() Conn
	Attach(handler InboundConnHandler)
	ConnectRequest() *Command
}

func NewInboundConn(ctx context.Context, c net.Conn, r *bufio.Reader, w *bufio.Writer,
	authHandler InboundAuthHandler, maxChannelNumber int) (InboundConn, error) {
	iConn := &inboundConn{
		authHandler: authHandler,
		status:      INBOUND_CONN_STATUS_CLOSE,
	}
	iConn.conn = NewConn(ctx, c, r, w, iConn, maxChannelNumber)
	return iConn, nil
}

type inboundConn struct {
	connectReq  *Command
	app         string
	handler     InboundConnHandler
	authHandler InboundAuthHandler
	conn        Conn
	status      uint
	err         error
	streams     sync.Map
	locker      sync.Mutex
}

// ////////////////////////////////////////////////////////////////////////////////////////
// ConnHandler
func (iconn *inboundConn) OnReceived(conn Conn, msg *Message) {
	if v, ok := iconn.streams.Load(msg.StreamID); ok {
		stream, _ := v.(*inboundStream)
		if !stream.Received(msg) {
			iconn.handler.OnReceived(iconn.conn, msg)
		}
	} else {
		iconn.handler.OnReceived(iconn.conn, msg)
	}
}

func (iconn *inboundConn) OnReceivedRtmpCommand(conn Conn, cmd *Command) {
	switch cmd.Name {
	case "connect":
		iconn.onConnect(cmd)
	case "createStream":
		iconn.onCreateStream(cmd)
	default:
		logger.Debugf("unhandled command: %+v", cmd)
	}
}

func (iconn *inboundConn) OnClosed(conn Conn) {
	iconn.status = INBOUND_CONN_STATUS_CLOSE
	iconn.handler.OnStatus(iconn)
}

// ////////////////////////////////////////////////////////////////////////////////////////
// InboundConn
func (iconn *inboundConn) Close() {
	iconn.streams.Range(func(key, value interface{}) bool {
		stream, _ := value.(*inboundStream)
		stream.Close()
		return true
	})
	time.Sleep(time.Second)
	iconn.status = INBOUND_CONN_STATUS_CLOSE
	iconn.conn.Close()
}

func (iconn *inboundConn) Status() (uint, error) {
	return iconn.status, iconn.err
}

func (iconn *inboundConn) Send(msg *Message) error {
	return iconn.conn.Send(msg)
}

func (iconn *inboundConn) Call(params ...interface{}) (err error) {
	return errors.New("To Be Continued...")
}

func (iconn *inboundConn) Conn() Conn {
	return iconn.conn
}

func (iconn *inboundConn) Attach(handler InboundConnHandler) {
	iconn.handler = handler
}

func (iconn *inboundConn) ConnectRequest() *Command {
	return iconn.connectReq
}

// ////////////////////////////////////////////////////////////////////////////////////////
// inboundConn
func (iconn *inboundConn) allocStream(stream *inboundStream) uint32 {
	iconn.locker.Lock()
	defer iconn.locker.Unlock()
	var i uint32 = 1
	for {
		if _, found := iconn.streams.Load(i); !found {
			stream.id = i
			iconn.streams.Store(i, stream)
			break
		}
		i++
	}
	return i
}

func (iconn *inboundConn) onConnect(cmd *Command) error {
	iconn.connectReq = cmd
	if cmd.Objects == nil || len(cmd.Objects) == 0 {
		logger.Errorf("onConnect() cmd.Object is nil")
		return iconn.sendConnectErrorResult(cmd)
	}
	params, ok := cmd.Objects[0].(amf.Object)
	if !ok {
		logger.Errorf("onConnect() parse cmd.Object failed")
		return iconn.sendConnectErrorResult(cmd)
	}

	if app, ok := params["app"]; !ok {
		logger.Errorf("onConnect() no app in cmd.Object")
		return iconn.sendConnectErrorResult(cmd)
	} else if iconn.app, ok = app.(string); !ok {
		logger.Errorf("onConnect() app is not a string")
		return iconn.sendConnectErrorResult(cmd)
	}

	var err error
	if iconn.authHandler.OnConnectAuth(iconn, cmd) {
		iconn.conn.SetWindowAcknowledgementSize()
		iconn.conn.SetPeerBandwidth(250000, SET_PEER_BANDWIDTH_DYNAMIC)
		iconn.conn.SetChunkSize(4096)
		err = iconn.sendConnectSucceededResult(cmd)
	} else {
		err = iconn.sendConnectErrorResult(cmd)
	}
	return err
}

func (iconn *inboundConn) onCreateStream(cmd *Command) {
	logger.Debugf(">>> inboundConn, onCreateStream")
	cs, err := iconn.conn.CreateMediaChunkStream()
	if err != nil {
		logger.Errorf("CreateStream() create media chunk stream err: %s", err.Error())
		return
	}
	stream := &inboundStream{
		conn:          iconn,
		chunkStreamID: cs.ID,
		closed:        false,
	}
	// TODO: use parent ctx
	stream.ctx, stream.cancel = context.WithCancel(context.Background())
	iconn.allocStream(stream)
	iconn.status = INBOUND_CONN_STATUS_CREATE_STREAM_OK
	iconn.handler.OnStatus(iconn)
	iconn.handler.OnStreamCreated(iconn, stream)
	if err = iconn.sendCreateStreamSuccessResult(cmd); err != nil {
		logger.Errorf("%+v", errors.Wrap(err, "inboundConn::sendCreateStreamSuccessResult"))
	}
}

func (iconn *inboundConn) releaseStream(streamID uint32) {
	iconn.streams.Delete(streamID)
}

func (iconn *inboundConn) onCloseStream(stream *inboundStream) {
	iconn.releaseStream(stream.id)
	iconn.handler.OnStreamClosed(iconn, stream)
}

func (iconn *inboundConn) sendConnectSucceededResult(req *Command) error {
	obj1 := make(amf.Object)
	obj1["fmsVer"] = fmt.Sprintf("FMS/%s", FMS_VERSION_STRING)
	obj1["capabilities"] = float64(255)
	obj2 := make(amf.Object)
	obj2["level"] = "status"
	obj2["code"] = RESULT_CONNECT_OK
	obj2["description"] = RESULT_CONNECT_OK_DESC
	return iconn.sendConnectRequest(req, "_result", obj1, obj2)
}

func (iconn *inboundConn) sendConnectErrorResult(req *Command) error {
	obj := make(amf.Object)
	obj["level"] = "status"
	obj["code"] = RESULT_CONNECT_REJECTED
	obj["description"] = RESULT_CONNECT_REJECTED_DESC
	return iconn.sendConnectRequest(req, "_error", nil, obj)
}

func (iconn *inboundConn) sendConnectRequest(req *Command, name string, obj1, obj2 interface{}) (err error) {
	cmd := &Command{
		IsFlex:        false,
		Name:          name,
		TransactionID: req.TransactionID,
		Objects:       make([]interface{}, 2),
	}
	cmd.Objects[0] = obj1
	cmd.Objects[1] = obj2

	buf := av.AcquirePacket()
	// TODO: error handle
	errPanic(cmd.Write(buf), "sendConnectRequest() create command")

	msg := &Message{
		ChunkStreamID: CS_ID_COMMAND,
		Type:          COMMAND_AMF0,
		Size:          uint32(buf.Len()),
		Buf:           buf,
	}
	return iconn.conn.Send(msg)
}

func (iconn *inboundConn) sendCreateStreamSuccessResult(req *Command) (err error) {
	cmd := &Command{
		IsFlex:        false,
		Name:          "_result",
		TransactionID: req.TransactionID,
		Objects:       make([]interface{}, 2),
	}
	cmd.Objects[0] = nil
	cmd.Objects[1] = int32(1)
	buf := av.AcquirePacket()
	if err = cmd.Write(buf); err != nil {
		return errors.WithMessage(err, "sendCreateStreamSuccessResult create command")
	}

	msg := &Message{
		ChunkStreamID: CS_ID_COMMAND,
		Size:          uint32(buf.Len()),
		Type:          COMMAND_AMF0,
		Buf:           buf,
	}
	return iconn.conn.Send(msg)
}

package rtmp

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"ken/lib/amf"
	"ken/lib/av"
)

const (
	OUTBOUND_CONN_STATUS_CLOSE            = uint(0)
	OUTBOUND_CONN_STATUS_HANDSHAKE_OK     = uint(1)
	OUTBOUND_CONN_STATUS_CONNECT          = uint(2)
	OUTBOUND_CONN_STATUS_CONNECT_OK       = uint(3)
	OUTBOUND_CONN_STATUS_CREATE_STREAM    = uint(4)
	OUTBOUND_CONN_STATUS_CREATE_STREAM_OK = uint(5)
)

type OutboundConnHandler interface {
	ConnHandler
	OnStatus(oconn OutboundConn)
	OnStreamCreated(oconn OutboundConn, stream OutboundStream)
}

type OutboundConn interface {
	Connect(params ...interface{}) error
	CreateStream() error
	Close()
	URL() string
	Status() (uint, error)
	Send(msg *Message) error
	Call(name string, params ...interface{}) error
	Conn() Conn
}

type outboundConn struct {
	url          string
	rtmpURL      RtmpURL
	status       uint
	err          error
	handler      OutboundConnHandler
	conn         Conn
	transactions sync.Map
	streams      sync.Map
}

func Dial(url string, handler OutboundConnHandler, maxChannelNumber int) (OutboundConn, error) {
	rtmpURL, err := ParseURL(url)
	if err != nil {
		return nil, err
	}
	var c net.Conn
	switch rtmpURL.protocol {
	case "rtmp":
		c, err = net.Dial("tcp", fmt.Sprintf("%s:%d", rtmpURL.host, rtmpURL.port))
	case "rtmps":
		c, err = tls.Dial("tcp", fmt.Sprintf("%s:%d", rtmpURL.host, rtmpURL.port), &tls.Config{InsecureSkipVerify: true})
	default:
		err = fmt.Errorf("unsupport protocol %s", rtmpURL.protocol)
	}
	if err != nil {
		return nil, err
	}

	if ipConn, ok := c.(*net.TCPConn); ok {
		ipConn.SetWriteBuffer(128 * 1024)
	}
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	timeout := 10 * time.Second
	if err = Handshake(c, r, w, timeout); err != nil {
		return nil, err
	}
	oconn := &outboundConn{
		url:     url,
		rtmpURL: rtmpURL,
		handler: handler,
		status:  OUTBOUND_CONN_STATUS_HANDSHAKE_OK,
	}
	oconn.handler.OnStatus(oconn)
	oconn.conn = NewConn(c, r, w, oconn, maxChannelNumber)
	return oconn, nil
}

func (oconn *outboundConn) Connect(params ...interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
			if oconn.err == nil {
				oconn.err = err
			}
		}
	}()

	buf := av.AcquirePacket()
	_, err = amf.WriteString(buf, "connect")
	errPanic(err, "'connect'")
	transactionID := oconn.conn.NewTransactionID()
	oconn.transactions.Store(transactionID, "connect")
	_, err = amf.WriteDouble(buf, float64(transactionID))
	errPanic(err, "write transactionId")
	_, err = amf.WriteObjectMarker(buf)
	errPanic(err, "write object marker")

	errPanic(writeString(buf, "app", oconn.rtmpURL.App()), "app")
	errPanic(writeString(buf, "flashVer", FLASH_PLAYER_VERSION_STRING), "flashVer")
	errPanic(writeString(buf, "tcUrl", oconn.url), "tcUrl")

	_, err = amf.WriteObjectName(buf, "fpad")
	errPanic(err, "write fpad name")
	_, err = amf.WriteBoolean(buf, false)
	errPanic(err, "write fpad value")

	errPanic(writeDouble(buf, "capabilities", DEFAULT_CAPABILITIES), "capabilities")
	errPanic(writeDouble(buf, "audioCodecs", DEFAULT_AUDIO_CODECS), "audioCodecs")
	errPanic(writeDouble(buf, "videoCodecs", DEFAULT_VIDEO_CODECS), "videoCodecs")
	errPanic(writeDouble(buf, "videoFunction", float64(1)), "videoFunction")

	_, err = amf.WriteObjectEndMarker(buf)
	errPanic(err, "write object end marker")

	for _, param := range params {
		_, err = amf.WriteValue(buf, param)
		errPanic(err, "write extended parameters")
	}

	msg := &Message{
		ChunkStreamID: CS_ID_COMMAND,
		Type:          COMMAND_AMF0,
		Size:          uint32(buf.Len()),
		Buf:           buf,
	}
	oconn.status = OUTBOUND_CONN_STATUS_CONNECT
	return oconn.conn.Send(msg)
}

func (oconn *outboundConn) Close() {
	oconn.streams.Range(func(k, v interface{}) bool {
		if stream, ok := v.(OutboundStream); ok {
			stream.Close()
		}
		return true
	})
	oconn.status = OUTBOUND_CONN_STATUS_CLOSE
	go func() {
		time.Sleep(time.Second)
		oconn.conn.Close()
	}()
}

func (oconn *outboundConn) CreateStream() (err error) {
	transactionID := oconn.conn.NewTransactionID()
	cmd := &Command{
		IsFlex:        false,
		Name:          "createStream",
		TransactionID: transactionID,
		Objects:       make([]interface{}, 1),
	}
	buf := av.AcquirePacket()
	if err = cmd.Write(buf); err != nil {
		return
	}
	oconn.transactions.Store(transactionID, "createStream")

	msg := &Message{
		ChunkStreamID: CS_ID_COMMAND,
		Type:          COMMAND_AMF0,
		Size:          uint32(buf.Len()),
		Buf:           buf,
	}
	return oconn.conn.Send(msg)
}

func (oconn *outboundConn) URL() string {
	return oconn.url
}

func (oconn *outboundConn) Status() (uint, error) {
	return oconn.status, oconn.err
}

func (oconn *outboundConn) Send(msg *Message) error {
	return oconn.conn.Send(msg)
}

func (oconn *outboundConn) Call(name string, params ...interface{}) (err error) {
	transactionID := oconn.conn.NewTransactionID()
	cmd := &Command{
		IsFlex:        false,
		Name:          name,
		TransactionID: transactionID,
		Objects:       make([]interface{}, 1+len(params)),
	}
	cmd.Objects[0] = nil
	for idx, param := range params {
		cmd.Objects[idx+1] = param
	}
	buf := av.AcquirePacket()
	if err = cmd.Write(buf); err != nil {
		return
	}
	oconn.transactions.Store(transactionID, name)
	msg := &Message{
		ChunkStreamID: CS_ID_COMMAND,
		Type:          COMMAND_AMF0,
		Size:          uint32(buf.Len()),
		Buf:           buf,
	}
	return oconn.conn.Send(msg)
}

func (oconn *outboundConn) Conn() Conn {
	return oconn.conn
}

func (oconn *outboundConn) OnReceived(conn Conn, msg *Message) {
	if v, ok := oconn.streams.Load(msg.StreamID); ok {
		if stream, ok := v.(OutboundStream); ok {
			if stream.Received(msg) {
				return
			}
		}
	}
	oconn.handler.OnReceived(conn, msg)
}

func (oconn *outboundConn) OnReceivedRtmpCommand(conn Conn, cmd *Command) {
	switch cmd.Name {
	case "_result":
		if v, ok := oconn.transactions.Load(cmd.TransactionID); ok {
			if transaction, ok := v.(string); ok {
				switch transaction {
				case "connect":
					if cmd.Objects != nil && len(cmd.Objects) > 1 {
						if info, ok := cmd.Objects[1].(amf.Object); ok {
							if code, ok := info["code"]; ok && code == RESULT_CONNECT_OK {
								oconn.conn.SetWindowAcknowledgementSize()
								oconn.status = OUTBOUND_CONN_STATUS_CONNECT_OK
								oconn.handler.OnStatus(oconn)
								oconn.status = OUTBOUND_CONN_STATUS_CREATE_STREAM
								oconn.CreateStream()
							}
						}
					}
				case "createStream":
					if cmd.Objects != nil && len(cmd.Objects) > 1 {
						if streamID, ok := cmd.Objects[1].(float64); ok {
							cs, err := oconn.conn.CreateMediaChunkStream()
							if err != nil {
								logger.Errorf("outboundConn::OnReceivedRtmpCommand() => CreateMediaChunkStream err: %s", err.Error())
								return
							}
							stream := &outboundStream{
								id:            uint32(streamID),
								conn:          oconn,
								chunkStreamID: cs.ID,
							}
							oconn.streams.Store(stream.ID(), stream)
							oconn.status = OUTBOUND_CONN_STATUS_CREATE_STREAM_OK
							oconn.handler.OnStatus(oconn)
							oconn.handler.OnStreamCreated(oconn, stream)
						}
					}
				}
				oconn.transactions.Delete(cmd.TransactionID)
			}
		}
	case "_error":
		logger.Errorf("result _error, transactionId: %d", cmd.TransactionID)
	case "onBWCheck":
	}
	oconn.handler.OnReceivedRtmpCommand(oconn.conn, cmd)
}

func (oconn *outboundConn) OnClosed(conn Conn) {
	oconn.status = OUTBOUND_CONN_STATUS_CLOSE
	oconn.handler.OnStatus(oconn)
	oconn.handler.OnClosed(conn)
}

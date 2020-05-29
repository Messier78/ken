package rtmp

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"ken/lib/amf"
	"ken/lib/av"
	"ken/service"
	"ken/types"
)

type InboundStreamHandler interface {
	OnPlayStart(stream InboundStream)
	OnPublishStart(stream InboundStream)
	OnReceived(msg *Message) bool
	OnReceiveAudio(stream InboundStream, on bool)
	OnReceiveVideo(stream InboundStream, on bool)
}

type InboundStream interface {
	Conn() InboundConn
	ID() uint32
	StreamName() string
	Close()
	Received(msg *Message) bool
	Attach(handler InboundStreamHandler)
	SendAudioData(data []byte, deltaTimestamp uint32) error
	SendVideoData(data []byte, deltaTimestamp uint32) error
	SendData(dataType uint8, data []byte, deltaTimestamp uint32) error
}

type inboundStream struct {
	ctx           context.Context
	cont          *types.Content
	handler       service.ServiceHandler
	id            uint32
	genID         int
	streamName    string
	keyString     string
	conn          *inboundConn
	chunkStreamID uint32
	bufferLength  uint32

	s       *av.Stream
	w       av.PacketWriter
	r       av.PacketReader
	f       *av.Packet
	metaObj amf.Object

	isPublisher bool
	idx         int64
	entry       int32
	closed      bool
}

func (stream *inboundStream) Conn() InboundConn {
	return stream.conn
}

func (stream *inboundStream) ID() uint32 {
	return stream.id
}

func (stream *inboundStream) StreamName() string {
	return stream.streamName
}

func (stream *inboundStream) Close() {
	var err error
	cmd := &Command{
		IsFlex:        true,
		Name:          "closeStream",
		TransactionID: 0,
		Objects:       make([]interface{}, 1),
	}
	cmd.Objects[0] = nil
	msg := NewMessage(stream.chunkStreamID, COMMAND_AMF3, stream.id, AUTO_TIMESTAMP, nil)
	if err = cmd.Write(msg.Buf); err != nil {
		return
	}
	conn := stream.conn.Conn()
	conn.Send(msg)
}

func (stream *inboundStream) Received(msg *Message) bool {
	if msg.Type == VIDEO_TYPE || msg.Type == AUDIO_TYPE {
		// if stream.w == nil {
		// 	stream.w, _ = stream.s.NewPacketWriter(stream.genID)
		// }
		msg.Buf.Type = msg.Type
		msg.Buf.Delta = msg.Timestamp
		stream.w.WritePacket(CodecPacket(msg.Buf))
		return true
	}
	if msg.Type == DATA_AMF0 {
		if stream.w == nil {
			stream.w, _ = stream.s.NewPacketWriter(stream.genID)
		}
		msg.Buf.IsMeta = true
		stream.w.WritePacket(msg.Buf)
		return true
	}
	var err error
	if msg.Type == COMMAND_AMF0 || msg.Type == COMMAND_AMF3 {
		cmd := &Command{}
		if msg.Type == COMMAND_AMF3 {
			cmd.IsFlex = true
			if _, err = msg.Buf.ReadByte(); err != nil {
				logger.Debugf("inboundStream received read first in flex command err: %s", err.Error())
				return true
			}
		}
		if cmd.Name, err = amf.ReadString(msg.Buf); err != nil {
			logger.Errorf("[%s] received AMF0 read name err: %s", stream.streamName, err.Error())
			return true
		}
		var transactionID float64
		if transactionID, err = amf.ReadDouble(msg.Buf); err != nil {
			logger.Errorf("[%s] Received() AMF0 read transactionId err: %s", stream.streamName, err.Error())
			return true
		}
		cmd.TransactionID = uint32(transactionID)
		var object interface{}
		for msg.Buf.Len() > 0 {
			if object, err = amf.ReadValue(msg.Buf); err != nil {
				logger.Errorf("[%s] Received() AMF0 read object err: %s", stream.streamName, err.Error())
				return true
			}
			cmd.Objects = append(cmd.Objects, object)
		}

		switch cmd.Name {
		case "play":
			return stream.onPlay(cmd)
		case "publish":
			return stream.onPublish(cmd)
		case "receiveAudio":
			return stream.onReceiveAduio(cmd)
		case "receiveVideo":
			return stream.onReceiveVideo(cmd)
		case "deleteStream":
			return stream.onDeleteStream(cmd)
		default:
			logger.Debugf("[%s] Received() unknown cmd: %+v", stream.streamName, cmd)
		}
	}
	return false
}

func (stream *inboundStream) Attach(handler InboundStreamHandler) {
	// stream.handler = handler
}

func (stream *inboundStream) SendAudioData(data []byte, deltaTimestamp uint32) error {
	msg := NewMessage(stream.chunkStreamID-4, AUDIO_TYPE, stream.id, AUTO_TIMESTAMP, data)
	msg.Timestamp = deltaTimestamp
	return stream.conn.Send(msg)
}

func (stream *inboundStream) SendVideoData(data []byte, deltaTimestamp uint32) error {
	msg := NewMessage(stream.chunkStreamID-4, VIDEO_TYPE, stream.id, AUTO_TIMESTAMP, data)
	msg.Timestamp = deltaTimestamp
	return stream.conn.Send(msg)
}

func (stream *inboundStream) SendData(dataType uint8, data []byte, deltaTimestamp uint32) error {
	var csid uint32
	switch dataType {
	case VIDEO_TYPE:
		csid = stream.chunkStreamID - 4
	case AUDIO_TYPE:
		csid = stream.chunkStreamID - 4
	default:
		csid = stream.chunkStreamID
	}
	msg := NewMessage(csid, dataType, stream.id, AUTO_TIMESTAMP, data)
	msg.Timestamp = deltaTimestamp
	return stream.conn.Send(msg)
}

func (stream *inboundStream) SendPacket(pkt *av.Packet) error {
	var csid uint32
	switch pkt.Type {
	case VIDEO_TYPE:
		csid = stream.chunkStreamID - 4
	case AUDIO_TYPE:
		csid = stream.chunkStreamID - 4
	default:
		csid = stream.chunkStreamID
	}
	msg := &Message{
		ChunkStreamID:     csid,
		Timestamp:         pkt.Delta,
		Type:              pkt.Type,
		StreamID:          stream.id,
		Buf:               pkt,
		AbsoluteTimestamp: pkt.Timestamp,
	}
	// if msg.Buf.Idx%100 == 0 {
	// 	logger.Debugf(">>> client absolute timestamp: %d", msg.AbsoluteTimestamp)
	// }
	msg.Size = uint32(msg.Buf.Len())

	return stream.conn.Send(msg)
}

func (stream *inboundStream) onPlay(cmd *Command) bool {
	if cmd.Objects == nil || len(cmd.Objects) < 2 || cmd.Objects[1] == nil {
		logger.Errorf("inboundStream::onPlay: command error 1 => %+v", cmd)
		return true
	}
	if streamName, ok := cmd.Objects[1].(string); !ok {
		logger.Errorf("inboundStream::onPlay command error 2 => %+v", cmd)
	} else {
		stream.streamName = streamName
	}
	u, err := url.Parse(stream.streamName)
	if err != nil {
		logger.Errorf("parse stream name failed: %s, err: %s", stream.streamName, err.Error())
		return false
	}
	// ----------- service ---------------
	cont := stream.cont
	cont.App = stream.conn.app
	cont.Name = u.Path
	cont.RawQuery = u.RawQuery
	cont.KeyString = stream.conn.app + u.Path
	cont.Status = 200
	stream.ctx = stream.handler.OnPlay(cont)
	if cont.Status != 200 {
		return false
	}
	// ----------- service ---------------
	stream.keyString = stream.cont.KeyString
	logger.Debugf("inboundStream::onPlay, key: %s", stream.keyString)
	stream.s = av.AttachToStream(stream.keyString)
	if stream.r = stream.s.NewPacketReader(); stream.r == nil {
		logger.Debugf("cannot get session reader!!!!!")
		return false
	}
	// Response
	stream.conn.conn.SetChunkSize(4096)
	stream.conn.conn.SendUserControlMessage(EVENT_STREAM_BEGIN)
	// stream.reset()
	stream.startPlay()
	stream.rtmpSampleAccess()

	logger.Debugf("---- client start play ----")
	go stream.play()
	return true
}

func (stream *inboundStream) onPublish(cmd *Command) bool {
	logger.Debugf(">> onPublish")
	if cmd.Objects == nil || len(cmd.Objects) < 2 || cmd.Objects[1] == nil {
		logger.Errorf("inboundStream::onPublish: command error 1 => %+v", cmd)
		return true
	}
	if streamName, ok := cmd.Objects[1].(string); !ok {
		logger.Errorf("inboundStream::onPublish command error 2 => %+v", cmd)
	} else {
		stream.streamName = streamName
	}
	u, err := url.Parse(stream.streamName)
	if err != nil || u == nil {
		logger.Errorf("parse stream name failed: %s", stream.streamName)
		return false
	}
	// ----------- service ---------------
	stream.cont.App = stream.conn.app
	stream.cont.Name = u.Path
	stream.cont.RawQuery = u.RawQuery
	stream.cont.KeyString = stream.conn.app + u.Path
	var status int
	stream.ctx, status = stream.handler.OnPublish(stream.cont)
	if status != 200 {
		return false
	}
	// ----------- service ---------------
	stream.keyString = stream.cont.KeyString
	stream.genID, _ = strconv.Atoi(u.Query().Get("genId"))
	stream.isPublisher = true
	stream.s = av.AttachToStream(stream.keyString)
	if stream.w == nil {
		stream.w, _ = stream.s.NewPacketWriter(stream.genID)
	}

	stream.startPublish()
	if err = stream.conn.Flush(); err != nil {
		logger.Errorf("start publish err: %s", err.Error())
		return false
	}
	return true
}

func (stream *inboundStream) onReceiveAduio(cmd *Command) bool {
	return true
}

func (stream *inboundStream) onReceiveVideo(cmd *Command) bool {
	logger.Debugf(">> onReceiveVideo")
	return true
}

func (stream *inboundStream) onDeleteStream(cmd *Command) bool {
	if stream.w != nil {
		stream.w.Close()
		stream.w = nil
	}
	stream.closed = true
	logger.Debugf(">> onDeleteStream, key: %s", stream.keyString)
	return true
}

func (stream *inboundStream) reset() {
	cmd := &Command{
		IsFlex:        false,
		Name:          "onStatus",
		TransactionID: 0,
		Objects:       make([]interface{}, 2),
	}
	cmd.Objects[0] = nil
	cmd.Objects[1] = amf.Object{
		"level":       "status",
		"code":        NETSTREAM_PLAY_RESET,
		"description": fmt.Sprintf("playing and resetting %s", stream.streamName),
		"details":     stream.streamName,
	}

	buf := av.AcquirePacket()
	errPanic(cmd.Write(buf), "inboundStream reset: create command")
	msg := &Message{
		ChunkStreamID: CS_ID_USER_CONTROL,
		Type:          COMMAND_AMF0,
		Size:          uint32(buf.Len()),
		Buf:           buf,
	}
	stream.conn.conn.Send(msg)
}

func (stream *inboundStream) startPlay() {
	cmd := &Command{
		IsFlex:        false,
		Name:          "onStatus",
		TransactionID: 0,
		Objects:       make([]interface{}, 2),
	}
	cmd.Objects[0] = nil
	cmd.Objects[1] = amf.Object{
		"level":       "status",
		"code":        NETSTREAM_PLAY_START,
		"description": fmt.Sprintf("startPlay playing %s", stream.streamName),
		"details":     stream.streamName,
	}

	buf := av.AcquirePacket()
	errPanic(cmd.Write(buf), "inboundStream startPlay: create command")
	msg := &Message{
		ChunkStreamID: CS_ID_USER_CONTROL,
		Type:          COMMAND_AMF0,
		Size:          uint32(buf.Len()),
		Buf:           buf,
	}
	stream.conn.conn.Send(msg)
}

func (stream *inboundStream) play() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("%+v", errors.Wrapf(r.(error), "panic while stream[%s] playing", stream.keyString))
		}
	}()
	if stream.r == nil {
		stream.r = stream.s.NewPacketReader()
		if stream.r == nil {
			logger.Errorf(">>>>>>>>>>>>>>>>>>>>>>> packet reader is nil")
		}
	}
	var err error
	for !stream.closed {
		err = stream.r.ReadPacket(stream.f)
		if err == av.ErrSendTooMuch {
			if netErr := stream.conn.Flush(); netErr != nil {
				logger.Errorf("flush data return error: %s", netErr.Error())
				return
			}
			time.Sleep(500 * time.Millisecond)
		} else if err == av.ErrNoPacketInCache {
			if netErr := stream.conn.Flush(); netErr != nil {
				logger.Errorf("flush data return error: %s", netErr.Error())
				return
			}
			time.Sleep(40 * time.Millisecond)
		}
		if stream.f.Buffer == nil {
			continue
		}
		if stream.f.IsMeta {
			if err = stream.onMetaData(stream.f); err != nil {
				logger.Errorf("send meta return error: %s", err.Error())
				return
			}
		}
		if stream.f.IsCodec {
			logger.Debugf("--->>> send codec data to client, len: %d", stream.f.Len())
		}
		// if err != nil {
		// 	logger.Infof("client read packet return err: %s", err.Error())
		// }
		// logger.Debugf("---- send data to client, type: %d, idx: %d, delta: %d, length: %d", stream.f.Type, stream.f.Idx, stream.f.Delta, stream.f.Len())
		if err = stream.SendPacket(stream.f); err != nil {
			logger.Errorf("send data return error: %s", err.Error())
			return
		}
	}
}

func (stream *inboundStream) startPublish() {
	cmd := &Command{
		IsFlex:        false,
		Name:          "onStatus",
		TransactionID: 0,
		Objects:       make([]interface{}, 2),
	}
	cmd.Objects[0] = nil
	cmd.Objects[1] = amf.Object{
		"level":       "status",
		"code":        NETSTREAM_PUBLISH_START,
		"description": "Start Publishing",
		"details":     stream.streamName,
	}

	buf := av.AcquirePacket()
	errPanic(cmd.Write(buf), "inboundStream startPlay: create command")
	msg := &Message{
		ChunkStreamID: CS_ID_USER_CONTROL,
		Type:          COMMAND_AMF0,
		Size:          uint32(buf.Len()),
		Buf:           buf,
	}
	stream.conn.conn.Send(msg)
}

func (stream *inboundStream) rtmpSampleAccess() {
	msg := NewMessage(CS_ID_USER_CONTROL, DATA_AMF0, 0, 0, nil)
	amf.WriteString(msg.Buf, "|RtmpSampleAccess")
	amf.WriteBoolean(msg.Buf, false)
	amf.WriteBoolean(msg.Buf, false)
	stream.conn.conn.Send(msg)
}

func (stream *inboundStream) onMetaData(f *av.Packet) error {
	logger.Infof(">>>> send meta data")
	msg := &Message{
		ChunkStreamID: CS_ID_USER_CONTROL,
		Type:          DATA_AMF0,
		Buf:           f,
	}
	return stream.conn.conn.Send(msg)
}

// //////////////////////////////////////////////////////////////////////
// Publisher
func (stream *inboundStream) GenID() int {
	return stream.genID
}

func (stream *inboundStream) Idx() int64 {
	return -1
}

// //////////////////////////////////////////////////////////////////////
// Player

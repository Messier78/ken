package rtmp

import (
	"fmt"

	"github.com/pkg/errors"

	"ken/lib/amf"
)

type OutboundStreamHandler interface {
	OnPlayStart(stream OutboundStream)
	OnPublishStart(stream OutboundStream)
}

type OutboundStream interface {
	OutboundPublishStream
	OutboundPlayStream
	ID() uint32
	Pause() error
	Resume() error
	Close()
	Received(msg *Message) bool
	Attach(handler OutboundStreamHandler)
	PublishAudioData(data []byte, deltaTimestamp uint32) error
	PublishVideoData(data []byte, deltaTimestamp uint32) error
	PublishData(dataType uint8, data []byte, deltaTimestamp uint32) error
	Call(name string, params ...interface{}) error
}

type OutboundPublishStream interface {
	Publish(name, t string) error
	SendAudioData(data []byte) error
	SendVideoData(data []byte) error
}

type OutboundPlayStream interface {
	Play(streamName string, start, duration *uint32, reset *bool) error
	Seek(offset uint32)
}

type outboundStream struct {
	id            uint32
	conn          OutboundConn
	chunkStreamID uint32
	handler       OutboundStreamHandler
	bufferLength  uint32
}

// ////////////////////////////////////////////////////////////////////////////////////////
// OutboundPublishStream
func (stream *outboundStream) Publish(streamName, publish string) (err error) {
	conn := stream.conn.Conn()
	cmd := &Command{
		IsFlex:        true,
		Name:          "publish",
		TransactionID: 0,
		Objects:       make([]interface{}, 3),
	}
	cmd.Objects[0] = nil
	cmd.Objects[1] = streamName
	if len(publish) > 0 {
		cmd.Objects[2] = publish
	} else {
		cmd.Objects[2] = nil
	}

	msg := NewMessage(stream.chunkStreamID, COMMAND_AMF3, stream.id, 0, nil)
	if err = cmd.Write(msg.Buf); err != nil {
		return
	}
	return conn.Send(msg)
}

func (stream *outboundStream) SendAudioData(data []byte) error {
	return errors.New("unsupported")
}

func (stream *outboundStream) SendVideoData(data []byte) error {
	return errors.New("unsupported")
}

// ////////////////////////////////////////////////////////////////////////////////////////
// OutboundPlayStream
func (stream *outboundStream) Play(streamName string, start, duration *uint32, reset *bool) (err error) {
	conn := stream.conn.Conn()
	cmd := &Command{
		IsFlex:        false,
		Name:          "play",
		TransactionID: 0,
		Objects:       make([]interface{}, 2),
	}
	cmd.Objects[0] = nil
	cmd.Objects[1] = streamName
	if start != nil {
		cmd.Objects = append(cmd.Objects, start)
	}
	zero := 0
	if duration != nil {
		if start == nil {
			cmd.Objects = append(cmd.Objects, &zero)
		}
		cmd.Objects = append(cmd.Objects, duration)
	}
	if reset != nil {
		if duration == nil {
			if start == nil {
				cmd.Objects = append(cmd.Objects, &zero)
			}
			cmd.Objects = append(cmd.Objects, &zero)
		}
		cmd.Objects = append(cmd.Objects, reset)
	}

	msg := NewMessage(stream.chunkStreamID, COMMAND_AMF0, stream.id, 0, nil)
	if err = cmd.Write(msg.Buf); err != nil {
		return
	}
	if err = conn.Send(msg); err != nil {
		return
	}

	if stream.bufferLength < MIN_BUFFER_LENGTH {
		stream.bufferLength = MIN_BUFFER_LENGTH
	}
	stream.conn.Conn().SetStreamBufferSize(stream.id, stream.bufferLength)
	return
}

func (stream *outboundStream) Seek(offset uint32) {
}

func (stream *outboundStream) ID() uint32 {
	return stream.id
}

func (stream *outboundStream) Pause() error {
	return fmt.Errorf("unsupported")
}

func (stream *outboundStream) Close() {
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

func (stream *outboundStream) Resume() error {
	return fmt.Errorf("unsupported")
}

func (stream *outboundStream) Received(msg *Message) bool {
	if msg.Type == VIDEO_TYPE || msg.Type == AUDIO_TYPE {
		return false
	}
	var err error
	if msg.Type == COMMAND_AMF0 || msg.Type == COMMAND_AMF3 {
		cmd := &Command{}
		if msg.Type == COMMAND_AMF3 {
			cmd.IsFlex = true
			if _, err = msg.Buf.ReadByte(); err != nil {
				logger.Debugf("%+v", errors.Wrap(err, "read first in flex command"))
				return true
			}
		}
		if cmd.Name, err = amf.ReadString(msg.Buf); err != nil {
			logger.Debugf("%+v", errors.Wrap(err, "AMF0 read name"))
			return true
		}

		var transactionID float64
		if transactionID, err = amf.ReadDouble(msg.Buf); err != nil {
			logger.Debugf("%+v", errors.Wrap(err, "read transactionId"))
			return true
		}
		cmd.TransactionID = uint32(transactionID)
		var object interface{}
		for msg.Buf.Len() > 0 {
			if object, err = amf.ReadValue(msg.Buf); err != nil {
				logger.Debugf("%+v", errors.Wrap(err, "AMF0 read object"))
				return true
			}
			cmd.Objects = append(cmd.Objects, object)
		}
		switch cmd.Name {
		case "onStatus":
			return stream.onStatus(cmd)
		case "onMetaData":
			return stream.onMetaData(cmd)
		case "onTimeCoordInfo":
			return stream.onTimeCoordInfo(cmd)
		default:
			logger.Errorf("%+v", errors.Wrapf(err, "unsupported command: %s", cmd.Name))
		}
	}
	return false
}

func (stream *outboundStream) Attach(handler OutboundStreamHandler) {
	stream.handler = handler
}

func (stream *outboundStream) PublishAudioData(data []byte, deltaTimestamp uint32) error {
	msg := NewMessage(stream.chunkStreamID, AUDIO_TYPE, stream.id, AUTO_TIMESTAMP, data)
	msg.Timestamp = deltaTimestamp
	return stream.conn.Send(msg)
}

func (stream *outboundStream) PublishVideoData(data []byte, deltaTimestamp uint32) error {
	msg := NewMessage(stream.chunkStreamID, VIDEO_TYPE, stream.id, AUTO_TIMESTAMP, data)
	msg.Timestamp = deltaTimestamp
	return stream.conn.Send(msg)
}

func (stream *outboundStream) PublishData(dataType uint8, data []byte, deltaTimestamp uint32) error {
	msg := NewMessage(stream.chunkStreamID, dataType, stream.id, AUTO_TIMESTAMP, data)
	msg.Timestamp = deltaTimestamp
	return stream.conn.Send(msg)
}

func (stream *outboundStream) Call(name string, params ...interface{}) (err error) {
	conn := stream.conn.Conn()
	cmd := &Command{
		IsFlex:        false,
		Name:          name,
		TransactionID: 0,
		Objects:       make([]interface{}, 1+len(params)),
	}
	cmd.Objects[0] = nil
	for idx, param := range params {
		cmd.Objects[idx+1] = param
	}

	msg := NewMessage(stream.chunkStreamID, COMMAND_AMF0, stream.id, 0, nil)
	if err = cmd.Write(msg.Buf); err != nil {
		return
	}
	if err = conn.Send(msg); err != nil {
		return
	}

	if stream.bufferLength < MIN_BUFFER_LENGTH {
		stream.bufferLength = MIN_BUFFER_LENGTH
	}
	stream.conn.Conn().SetStreamBufferSize(stream.id, stream.bufferLength)
	return
}

// ////////////////////////////////////////////////////////////////////////////////////////
func (stream *outboundStream) onStatus(cmd *Command) bool {
	var code string
	if len(cmd.Objects) > 1 {
		if obj, ok := cmd.Objects[1].(amf.Object); ok {
			if v, ok := obj["code"]; ok {
				code = v.(string)
			}
		}
	}
	switch code {
	case NETSTREAM_PLAY_START:
		if stream.handler != nil {
			stream.handler.OnPlayStart(stream)
		}
	case NETSTREAM_PUBLISH_START:
		if stream.handler != nil {
			stream.handler.OnPublishStart(stream)
		}
	}
	return false
}

func (stream *outboundStream) onMetaData(cmd *Command) bool {
	return false
}

func (stream *outboundStream) onTimeCoordInfo(cmd *Command) bool {
	return false
}

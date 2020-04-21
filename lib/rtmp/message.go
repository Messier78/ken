package rtmp

import (
	"ken/lib/av"
)

type Message struct {
	ChunkStreamID     uint32
	Timestamp         uint32
	Size              uint32
	Type              uint8
	StreamID          uint32
	Buf               *av.Packet
	IsInbound         bool
	AbsoluteTimestamp uint32
}

func NewMessage(csi uint32, t uint8, sid uint32, ts uint32, data []byte) *Message {
	msg := &Message{
		ChunkStreamID:     csi,
		Type:              t,
		StreamID:          sid,
		Timestamp:         ts,
		AbsoluteTimestamp: ts,
		Buf:               av.AcquirePacket(),
	}
	if data != nil {
		msg.Buf.Write(data)
		msg.Size = uint32(len(data))
	}
	return msg
}

func (msg *Message) Remain() uint32 {
	if msg.Buf == nil {
		return msg.Size
	}
	return msg.Size - uint32(msg.Buf.Len())
}

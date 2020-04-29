package av

import (
	"bytes"
	"sync"
)

var (
	pool *sync.Pool
)

func init() {
	pool = &sync.Pool{
		New: func() interface{} {
			return newPacket()
		},
	}
}

// Packet
type Packet struct {
	bytes.Buffer

	Idx int64
	// Packet Type as av type
	Type       uint8
	IsKeyFrame bool
	IsCodec    bool
	Timestamp  uint32
	Delta      uint32
}

func newPacket() *Packet {
	return &Packet{}
}

func AcquirePacket() *Packet {
	v, _ := pool.Get().(*Packet)
	v.Reset()
	return v
}

func ReleasePacket(pkt *Packet) {
	pool.Put(pkt)
}

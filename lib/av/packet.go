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

type Packet struct {
	bytes.Buffer
}

func newPacket() *Packet {
	return &Packet{}
}

func AcquirePacket() *Packet {
	v, _ := pool.Get().(*Packet)
	return v
}

func ReleasePacket(pkt *Packet) {
	pool.Put(pkt)
}

type Encoder interface {
	Write(pkt *Packet) error
}

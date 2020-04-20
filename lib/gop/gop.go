package gop

import (
	"container/ring"
	"sync"

	"ken/lib/av"
)

const (
	defaultRingLength = 4096
)

type Cache struct {
	Idx          int64
	r            *ring.Ring
	length       int
	avc          *av.Packet
	aac          *av.Packet
	lastKeyFrame *ring.Ring

	cond *sync.Cond
}

func NewGopCache() *Cache {
	return &Cache{
		Idx:    0,
		r:      ring.New(defaultRingLength),
		length: defaultRingLength,
		cond:   sync.NewCond(&sync.Mutex{}),
	}
}

func (c *Cache) GetStartPos() (avc, aac *av.Packet, r *ring.Ring, cond *sync.Cond) {
	return c.avc, c.aac, c.lastKeyFrame, c.cond
}

func (c *Cache) WritePacket(pkt *av.Packet, isKeyFrame bool) {
	c.r.Value = pkt
	if isKeyFrame {
		c.lastKeyFrame = c.r
	}
	c.r = c.r.Next()
	if c.r.Value != nil {
		if v, ok := c.r.Value.(*av.Packet); ok {
			av.ReleasePacket(v)
		}
		c.r.Value = nil
	}
	c.Idx++
	c.cond.Broadcast()
}

func (c *Cache) WriteVideoPacket(pkt *av.Packet) {
	c.r.Value = pkt
	c.r = c.r.Next()
	c.avc = pkt
	c.Idx++
}

func (c *Cache) WriteAudioPacket(pkt *av.Packet) {
	c.r.Value = pkt
	c.r = c.r.Next()
	c.aac = pkt
	c.Idx++
}

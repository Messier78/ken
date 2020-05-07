package av

import (
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
)

type Cache struct {
	// Idx++ when a packet written
	Idx int64
	// LatestTimestamp added with packet.deltaTimestamp
	// when packet written
	LatestTimestamp uint32

	// CodecVersion++ when aac/avc packet written
	CodecVersion uint32
	AAC          *Packet
	AVC          *Packet
	MetaVersion  uint32
	Meta         *Packet
	// AudioOnly
	// set this to false when video key frame received
	AudioOnly bool

	// gop queue head
	gopStart *gop
	// latest gop
	gPos            *gop
	node            *pktNode
	durationInCache uint32

	// gop cache writer
	w                *packetWriter
	writerSwapLocker *sync.RWMutex
	writers          sync.Map
	cond             *sync.Cond
}

func NewCache() *Cache {
	c := &Cache{
		AudioOnly: true,
		gPos: &gop{
			idx:      -1,
			node:     &pktNode{},
			duration: conf.AudioOnlyGopDuration + 1,
		},
		writerSwapLocker: &sync.RWMutex{},
		cond:             sync.NewCond(&sync.Mutex{}),
	}
	c.gopStart = c.gPos
	return c
}

// Write to gop cache
func (c *Cache) Write(f *Packet) {
	// Codec
	if f.IsCodec {
		c.gPos = c.gPos.WriteInNewGop(f, 0)
		if f.Type == AUDIO_TYPE {
			c.AAC = f
			atomic.AddUint32(&c.CodecVersion, 1)
		} else if f.Type == VIDEO_TYPE {
			c.AVC = f
			atomic.AddUint32(&c.CodecVersion, 1)
		}
		return
	}
	// AudioOnly
	if c.AudioOnly {
		if f.Type == AUDIO_TYPE {
			if c.gPos.duration > conf.AudioOnlyGopDuration {
				c.gPos = c.gPos.WriteInNewGop(f, c.Idx+1)
			} else {
				c.gPos.Write(f)
			}
			c.Idx++
			c.cond.Broadcast()
			return
		} else {
			// drop video frame until key frame
			return
		}
	}

	if f.IsKeyFrame {
		c.AudioOnly = false
		c.gPos = c.gPos.WriteInNewGop(f, c.Idx+1)
	} else {
		c.gPos.Write(f)
	}
	c.Idx++
	c.cond.Broadcast()
}

// NewPacketWriter returns a PacketWriter interface that implements
// the WritePacket and Close methods
// Publisher can write packet to Cache or packetWriter
func (c *Cache) NewPacketWriter(genID int) (PacketWriter, error) {
	// use value 1 to occupy this genID
	if _, ok := c.writers.LoadOrStore(genID, 1); ok {
		return nil, errors.New("stream with the same genId existed")
	}
	w := &packetWriter{
		r:          newPacketRing(conf.RingSize),
		swapLocker: c.writerSwapLocker.RLocker(),
	}
	c.writers.Store(genID, w)
	// write into self ring by default
	w.handler = w

	// overflow with higher genId writer
	if c.w == nil {
		c.w = w
		c.w.handler = c
	} else if w.genID > c.w.genID {
		c.writerSwapLocker.Lock()
		c.w.handler = c.w
		c.w = w
		c.w.handler = c
		c.writerSwapLocker.Unlock()
	}
	return w, nil
}

// ClosePacketWriter
func (c *Cache) ClosePacketWriter(w *packetWriter) {
	// TODO: swap writer
}

// PacketReader
func (c *Cache) NewPacketReader() PacketReader {
	r := &packetReader{
		cache: c,
		cond:  c.cond,
	}
	return r
}

// getStartNode return packet node which begins
// with avc/aac if exists and key frame packet
func (c *Cache) getStartNode() *pktNode {
	g := c.gPos
	var p, nnode *pktNode
	nnode = &pktNode{}
	p = nnode

	return p.next
}

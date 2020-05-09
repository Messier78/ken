package av

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

type Cache struct {

	// Idx++ when a packet written
	Idx int64
	// LatestTimestamp added with packet.deltaTimestamp
	// when packet written
	LatestTimestamp uint32

	// CodecVersion++ when aac/avc packet written
	CodecVersion    uint32
	codecSwapLocker *sync.RWMutex
	AAC             *Packet
	AVC             *Packet
	MetaVersion     uint32
	Meta            *Packet
	// AudioOnly
	// set this to false when video key frame received
	AudioOnly   bool
	prevIsCodec bool

	// idx of the first frame in cache
	StartIdx       int64
	codecNodeStart *pktNode
	codecNode      *pktNode
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

	keyString string
}

func NewCache() *Cache {
	c := &Cache{
		codecSwapLocker: &sync.RWMutex{},
		AudioOnly:       true,
		prevIsCodec:     false,
		codecNodeStart: &pktNode{
			f: &Packet{
				Idx: -1,
			},
		},
		gPos: &gop{
			idx:       0,
			nodeStart: &pktNode{},
			duration:  conf.AudioOnlyGopDuration + 1,
		},
		writerSwapLocker: &sync.RWMutex{},
		cond:             sync.NewCond(&sync.Mutex{}),
	}
	c.codecNode = c.codecNodeStart
	c.gPos.node = c.gPos.nodeStart
	c.gopStart = c.gPos
	return c
}

func (c *Cache) SetID(id string) {
	c.keyString = id
	go c.monitor()
}

// Write to gop cache
func (c *Cache) Write(f *Packet) {
	f.Timestamp = c.LatestTimestamp
	// Codec
	if f.IsCodec {
		f.Idx = c.Idx
		// if c.gPos.idx < 0 {
		// 	c.gPos = c.gPos.WriteInNewGop(f, 0)
		// } else {
		// 	c.gPos.Write(f)
		// }
		if c.gPos.idx > 0 {
			c.gPos.Write(f)
		}
		c.codecNode.next = &pktNode{f: f}
		c.codecNode = c.codecNode.next
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
	if c.AudioOnly && !f.IsKeyFrame {
		if f.Type == AUDIO_TYPE {
			f.Idx = c.Idx + 1
			if c.gPos.duration > conf.AudioOnlyGopDuration {
				c.LatestTimestamp += c.gPos.duration
				c.gPos = c.gPos.WriteInNewGop(f, c.Idx+1)
				c.swapGopStart()
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

	f.Idx = c.Idx + 1
	if f.IsKeyFrame {
		c.AudioOnly = false
		c.LatestTimestamp += c.gPos.duration
		c.gPos = c.gPos.WriteInNewGop(f, c.Idx+1)
		c.swapGopStart()
	} else {
		c.gPos.Write(f)
	}
	c.Idx++
	c.cond.Broadcast()
}

func (c *Cache) swapGopStart() {
	pos := c.gopStart
	// keep the latest gop always
	gpos := c.gPos.prev
	for ; pos != gpos && pos != gpos.prev; pos = pos.next {
		if pos.timestamp+conf.DelayTime < c.LatestTimestamp {
			logger.Debugf("---------- drop gop, idx: %d", pos.idx)
			pos.next.prev = nil
			c.gopStart = pos.next
		}
	}
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
	logger.Debugf("new reader from Cache...")
	r := &packetReader{
		cache: c,
		cond:  c.cond,
	}
	r.node = c.getStartNode()
	if r.node == nil {
		return nil
	}
	node := r.node
	for i := 0; i < 5; i++ {
		logger.Debugf("||||---- frame idx: %d", node.f.Idx)
		if node.next != nil {
			node = node.next
		} else {
			break
		}
	}
	return r
}

// getStartNode return packet node which begins
// with avc/aac if exists and key frame packet
func (c *Cache) getStartNode() *pktNode {
	if c.gopStart.next == nil {
		// TODO: relay
		c.cond.L.Lock()
		c.cond.Wait()
		c.cond.L.Unlock()
	}
	// c.codecSwapLocker.RLock()
	// defer c.codecSwapLocker.RUnlock()
	pos := c.gopStart
	if pos.idx < 1 {
		pos = pos.next
	}
	var avc, aac *Packet
	node := c.codecNodeStart
	if node.f.Idx < 0 && node.next != nil {
		node = node.next
	}
	for node != nil {
		logger.Infof(">>> codec idx: %d", node.f.Idx)
		if node.f.Idx > pos.idx {
			break
		}
		if node.f.IsAAC {
			aac = node.f
		} else {
			avc = node.f
		}
		node = node.next
	}
	pnode := &pktNode{}
	nnode := pnode
	if avc != nil {
		pnode.next = &pktNode{
			f: avc,
		}
		pnode = pnode.next
	}
	if aac != nil {
		pnode.next = &pktNode{
			f: aac,
		}
		pnode = pnode.next
	}
	pnode.next = &pktNode{
		f:    pos.nodeStart.f,
		next: pos.nodeStart.next,
	}
	return nnode.next
}

// monitor
func (c *Cache) monitor() {
	for {
		select {
		case <-time.After(5 * time.Second):
			logger.Debugf(">>> [%s] idx: %d, cache.Idx: %d, timestamp: %d", c.keyString, c.Idx, c.LatestTimestamp)
		}
	}
}

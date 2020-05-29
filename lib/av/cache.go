package av

import (
	"sync"
	"time"

	"github.com/pkg/errors"
)

// TODO: drop codec
type Cache struct {
	// Idx++ when a packet written
	Idx int64
	// LatestTimestamp added with packet.deltaTimestamp
	// when packet written
	LatestTimestamp uint32
	// Avaliable is not 0 if avaliable writer exists
	Avaliable int32

	codecSwapLocker *sync.RWMutex
	/*
		// CodecVersion++ when aac/avc packet written
		CodecVersion    uint32
		AAC             *Packet
		AVC             *Packet
		MetaVersion     uint32
		Meta            *Packet
	*/
	// AudioOnly
	// set this to false when video key frame received
	AudioOnly bool
	// TimestampFix
	TimestampFix bool
	maxDelta     uint32

	// idx of the first frame in cache
	StartIdx       int64
	codecNodeStart *pktNode
	codecNode      *pktNode
	metaNodeStart  *pktNode
	metaNode       *pktNode
	// gop queue head
	gopStart *gop
	// latest gop
	gPos            *gop
	node            *pktNode
	durationInCache uint32

	// gop cache writer
	w                *packetWriter
	genID            int
	writerSwapLocker *sync.RWMutex
	writers          map[int]*packetWriter
	cond             *sync.Cond

	noDataReceivedCnt uint32
	informationCnt    uint32

	keyString string
	errStatus error
	conf      *Config
}

func NewCache() *Cache {
	f := AcquirePacket()
	f.Idx = -1
	c := &Cache{
		codecSwapLocker: &sync.RWMutex{},
		AudioOnly:       true,
		TimestampFix:    true,
		codecNodeStart:  &pktNode{f: f},
		metaNodeStart:   &pktNode{f: f},
		gPos: &gop{
			idx:       0,
			nodeStart: &pktNode{},
		},
		writerSwapLocker: &sync.RWMutex{},
		writers:          make(map[int]*packetWriter),
		cond:             sync.NewCond(&sync.Mutex{}),
		conf:             initConfig(),
	}
	c.gPos.duration = c.conf.AudioOnlyGopDuration + 1
	c.codecNode = c.codecNodeStart
	c.metaNode = c.metaNodeStart
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
	// logger.Infof(">>> delta: %d", f.Delta)
	if c.TimestampFix {
		c.fixTimestamp(f)
	}
	c.noDataReceivedCnt = 0
	f.Timestamp = c.LatestTimestamp
	// Codec
	if f.IsCodec {
		f.Idx = c.Idx
		if c.gPos.idx > 0 {
			c.gPos.Write(f)
		}
		c.codecNode.next = &pktNode{f: f}
		c.codecNode = c.codecNode.next
		return
	} else if f.IsMeta {
		f.Idx = c.Idx
		if c.gPos.idx > 0 {
			c.gPos.Write(f)
		}
		c.metaNode.next = &pktNode{f: f}
		c.metaNode = c.metaNode.next
		return
	}

	// AudioOnly
	if c.AudioOnly && !f.IsKeyFrame {
		if f.Type == AUDIO_TYPE {
			f.Idx = c.Idx + 1
			if c.gPos.duration > c.conf.AudioOnlyGopDuration {
				c.LatestTimestamp += c.gPos.duration
				c.gPos = c.gPos.WriteInNewGop(f, c.Idx+1)
				c.swapGopStart()
			} else {
				c.gPos.Write(f)
			}
			c.Idx++
			// c.cond.Broadcast()
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
	// c.cond.Broadcast()
}

func (c *Cache) swapGopStart() {
	pos := c.gopStart
	// keep the latest gop always
	gpos := c.gPos.prev
	for ; pos != gpos && pos != gpos.prev; pos = pos.next {
		if pos.timestamp+c.conf.DelayTime < c.LatestTimestamp {
			// logger.Debugf("---------- drop gop, idx: %d", pos.idx)
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
	c.writerSwapLocker.Lock()
	defer c.writerSwapLocker.Unlock()
	if _, ok := c.writers[genID]; ok {
		return nil, errors.New("stream with the same genId existed")
	}
	logger.Infof("new writer with genId %d", genID)
	w := &packetWriter{
		genID:      genID,
		r:          newPacketRing(c.conf.RingSize),
		swapLocker: c.writerSwapLocker.RLocker(),
	}
	c.writers[genID] = w
	// write into self ring by default
	w.handler = w

	// overflow with higher genId writer
	if c.w == nil {
		c.w = w
		c.w.handler = c
		c.genID = w.genID
	} else if w.genID > c.genID {
		c.w.handler = c.w
		c.w = w
		c.w.handler = c
		c.genID = w.genID
	}
	return w, nil
}

// ClosePacketWriter
func (c *Cache) ClosePacketWriter(w *packetWriter) {
	c.writerSwapLocker.Lock()
	defer c.writerSwapLocker.Unlock()
	genID := w.genID
	delete(c.writers, genID)
	// swap writer
	if c.genID == genID {
		c.w = nil
		var w *packetWriter
		var id int
		for iid, ww := range c.writers {
			if w == nil || iid > id {
				w = ww
				id = iid
			}
		}
		if w != nil {
			c.genID = id
			c.w = w
			c.w.handler = c
			if c.w.avc != nil {
				c.Write(c.w.avc)
			}
			if c.w.aac != nil {
				c.Write(c.w.aac)
			}
			f := c.w.lastKeyFrame.Value
			if f != nil {
				// key frame pos is covered
				if !f.IsKeyFrame {
					return
				}
				c.Write(f)
				idx := f.Idx
				r := c.w.lastKeyFrame
				for {
					r = r.Next()
					f = r.Value
					if f != nil && f.Idx > idx {
						c.Write(f)
						continue
					}
					return
				}
			}
		}
	}
}

// TODO: 增加参数决定这个reader是否需要LowLatency
// PacketReader
func (c *Cache) NewPacketReader() PacketReader {
	logger.Debugf("new reader from Cache...")
	r := &packetReader{
		cache:                c,
		packetUnavaliableCnt: 0,
	}
	for {
		r.node, r.startTime = c.getStartNode()
		// wait for relay
		if r.node == nil && r.packetUnavaliableCnt < 6 {
			r.packetUnavaliableCnt++
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	if r.node == nil {
		return nil
	}
	r.startSysTime = time.Now()

	return r
}

// getStartNode return packet node which begins
// with avc/aac if exists and key frame packet
func (c *Cache) getStartNode() (*pktNode, uint32) {
	// logger.Infof("Cache, getStartNode...")
	if c.gopStart.next == nil {
		// TODO: relay
		return nil, 0
	}
	c.codecSwapLocker.RLock()
	defer c.codecSwapLocker.RUnlock()
	pos := c.gopStart
	if pos.idx < 1 {
		pos = pos.next
	}
	// link the latest meta/avc/aac packet to reader
	var avc, aac, meta *Packet
	node := c.metaNodeStart
	for node != nil {
		if node.f.Idx > pos.idx {
			break
		}
		meta = node.f
		node = node.next
	}
	node = c.codecNodeStart
	if node.f.Idx < 0 && node.next != nil {
		node = node.next
	}
	for node != nil {
		// logger.Infof(">>> codec idx: %d", node.f.Idx)
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
	if meta != nil && meta.Idx >= 0 {
		pnode.next = &pktNode{
			f: meta,
		}
		// logger.Debugf(">>> link meta, idx: %d, len: %d", meta.Idx, meta.Len())
		pnode = pnode.next
	}
	if avc != nil {
		pnode.next = &pktNode{
			f: avc,
		}
		// logger.Debugf(">>> link AVC, idx: %d, len: %d", avc.Idx, avc.Len())
		pnode = pnode.next
	}
	if aac != nil {
		pnode.next = &pktNode{
			f: aac,
		}
		// logger.Debugf(">>> link AAC, idx: %d, len: %d", aac.Idx, aac.Len())
		pnode = pnode.next
	}
	pnode.next = &pktNode{
		f:    pos.nodeStart.f,
		next: pos.nodeStart.next,
	}
	logger.Debugf(">>> link first key frame, idx: %d", pos.nodeStart.f.Idx)

	return nnode.next, pos.nodeStart.f.Timestamp
}

func (c *Cache) fixTimestamp(f *Packet) {
	if f.Delta < c.conf.MaxAvaliableDelta && f.Delta > c.maxDelta {
		c.maxDelta = f.Delta
	}
	// logger.Debugf("[%s] frame delta: %d", c.keyString, f.Delta)
	if f.Delta > c.conf.MaxDelta {
		logger.Debugf("[%s] set frame delta from %d to %d", c.keyString, f.Delta, c.maxDelta)
		f.Delta = c.maxDelta
	}
}

// monitor
func (c *Cache) monitor() {
	for {
		select {
		case <-time.After(time.Second):
			c.noDataReceivedCnt++
			if c.noDataReceivedCnt > c.conf.DropIdleWriter {
			}
			c.informationCnt++
			if c.informationCnt > 4 {
				c.informationCnt = 0
				logger.Infof(">>> [%s] cache.Idx: %d, timestamp: %d", c.keyString, c.Idx, c.LatestTimestamp)
			}
		}
	}
}

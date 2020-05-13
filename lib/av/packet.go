package av

import (
	"bytes"
	"sync"
	"time"
)

// Packet
type Packet struct {
	*bytes.Buffer

	Idx int64
	// Packet Type as av type
	Type       uint8
	IsKeyFrame bool
	IsCodec    bool
	IsAAC      bool
	IsMeta     bool
	Timestamp  uint32
	Delta      uint32
}

func newPacket() *Packet {
	return &Packet{}
}

func AcquirePacket() *Packet {
	// v, _ := pool.Get().(*Packet)
	// v.Reset()
	return &Packet{Buffer: &bytes.Buffer{}}
}

func ReleasePacket(pkt *Packet) {
}

// PacketReader
type PacketReader interface {
	ReadPacket(ff *Packet) error
}

type packetReader struct {
	idx          int64
	avc          *Packet
	aac          *Packet
	codecVersion uint32
	metaVersion  uint32
	// timestamp of this reader has been played
	timestamp uint32
	// absoulte timestamp of the first frame
	startTime uint32
	//
	startSysTime time.Time

	cache *Cache
	node  *pktNode
	cond  *sync.Cond
}

// ReadPacket
func (r *packetReader) ReadPacket(ff *Packet) (err error) {
	if r.idx < r.cache.StartIdx {
		r.reset()
	}
	var f *Packet
	defer func() {
		r.fixPacket(f, ff)
		t := r.startSysTime.Add(time.Duration(r.timestamp) * time.Millisecond)
		if t.After(time.Now().Add(r.cache.conf.ClientDuration)) {
			time.Sleep(500 * time.Millisecond)
		}
	}()

	if r.node.next != nil {
		f = r.node.f
		r.node = r.node.next
		return nil
	}

	r.cond.L.Lock()
	r.cond.Wait()
	r.cond.L.Unlock()
	// time.Sleep(40 * time.Millisecond)

	if r.node.next == nil {
		return r.cache.errStatus
	}
	f = r.node.f
	r.node = r.node.next
	return nil
}

// fixPacket
// fix timestamp base on packetReader
func (r *packetReader) fixPacket(f *Packet, ff *Packet) {
	if f == nil {
		ff.Buffer = nil
		return
	}

	ff.Buffer = bytes.NewBuffer(f.Buffer.Bytes())
	ff.Idx = f.Idx
	ff.Type = f.Type
	ff.IsCodec = f.IsCodec
	ff.IsMeta = f.IsMeta
	ff.Delta = f.Delta
	ff.Timestamp = r.startTime + r.timestamp
	r.timestamp += f.Delta
}

// reset
func (r *packetReader) reset() {
	ts := r.cache.LatestTimestamp
	var p, lastKeyNode *pktNode
	nnode := &pktNode{}
	p = nnode
	node := r.node
	for node.next != nil {
		// link all codec/meta packet to node
		if node.f.IsCodec || node.f.IsMeta {
			nnode.next = &pktNode{
				f: node.f,
			}
			nnode = nnode.next
		}
		if node.f.IsKeyFrame {
			lastKeyNode = node
		}
		if node.f.Timestamp+r.cache.conf.DelayTime > ts {
			// only if the publisher is AUDIO ONLY
			if lastKeyNode == nil {
				lastKeyNode = node
			}
			break
		}
		node = node.next
	}
	nnode.next = lastKeyNode
	r.node = p.next
}

// PacketWriter
type PacketWriter interface {
	WritePacket(f *Packet) error
	Close()
}

type Writer interface {
	Write(f *Packet)
}

type packetWriter struct {
	idx     int64
	genID   int
	cache   *Cache
	handler Writer

	avc, aac     *Packet
	lastKeyFrame *packetRing
	r            *packetRing

	swapLocker sync.Locker
}

func (w *packetWriter) WritePacket(f *Packet) error {
	w.swapLocker.Lock()
	defer w.swapLocker.Unlock()
	// store packet into ring in case stream overflowed
	w.r.Value = f
	if f.IsCodec {
		if f.Type == AUDIO_TYPE {
			w.aac = f
		} else {
			w.avc = f
		}
	} else if f.IsKeyFrame {
		w.lastKeyFrame = w.r
	}

	w.handler.Write(f)
	w.r = w.r.Next()
	return nil
}

// Write
func (w *packetWriter) Write(f *Packet) {
	f.Idx = w.idx + 1
	w.idx++
}

func (w *packetWriter) Close() {
	w.cache.ClosePacketWriter(w)
}

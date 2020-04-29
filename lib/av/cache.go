package av

import (
	"container/ring"
	"sync"
)

const (
	defaultRingLength = 8192
)

type Cache struct {
	Idx          int64
	r            *ring.Ring
	length       int
	avc          *Packet
	aac          *Packet
	lastKeyFrame *ring.Ring

	// writers stored with genId
	// writer with the same genId will be ignored
	writers sync.Map
	cond    *sync.Cond
}

func NewGopCache() *Cache {
	return &Cache{
		Idx:    0,
		r:      ring.New(defaultRingLength),
		length: defaultRingLength,
		cond:   sync.NewCond(&sync.Mutex{}),
	}
}

func (c *Cache) GetStartPos() (avc, aac *Packet, r *ring.Ring, cond *sync.Cond) {
	return c.avc, c.aac, c.lastKeyFrame, c.cond
}

func (c *Cache) WritePacket(f *Packet) {
	c.r.Value = f
	defer func() {
		c.r = c.r.Next()
		if c.r.Value != nil {
			if v, ok := c.r.Value.(*Packet); ok {
				ReleasePacket(v)
			}
		}
	}()

	if f.IsCodec {
		if f.Type == VIDEO_TYPE {
			logger.Debugf("Write AVC packet")
			c.avc = f
		} else {
			logger.Debugf("Write AAC packet")
			c.aac = f
		}
		f.Idx = c.Idx
		c.Idx++
		return
	}
	if f.IsKeyFrame {
		c.lastKeyFrame = c.r
	}

	f.Idx = c.Idx
	c.Idx++
	c.cond.Broadcast()
}

func (c *Cache) Broadcast() {
	c.cond.Broadcast()
}

func (c *Cache) Reset() {
	c.Idx = 0
}

// Writer of Cache
// writer with higher genId can write packet into
// cache queue
func (c *Cache) NewWriter(genID int) Writer {
	return &cacheWriter{}
}

type cacheWriter struct {
}

func (cw *cacheWriter) WritePacket(f *Packet) error {
	return nil
}

// Reader of Cache
func (c *Cache) NewReader() Reader {
	return &cacheReader{
		cond: c.cond,
	}
}

type cacheReader struct {
	cond *sync.Cond
}

func (cr *cacheReader) ReadPacket() (*Packet, error) {
	return nil, nil
}

// gop
// a list of frames begin with key frame
// copy code from ring/ring.go
package av

// |-----|    |-----|    |-----|    |-----|
// | gop | -> | gop | -> | gop | -> | gop |
// |-----|    |-----|    |-----|    |-----|
//    ↓          ↓          ↓          ↓
//   pkt        pkt        pkt        pkt
//    ↓          ↓          ↓          ↓
//   pkt        pkt        pkt        pkt
//    ↓          ↓          ↓          ↓
//   pkt        pkt        pkt        pkt
//    ↓          ↓          ↓          ↓
//   pkt        pkt        pkt        pkt

type gop struct {
	// timestamp of first frame
	timestamp uint32
	// idx of first frame
	idx int64

	next, prev *gop
	node       *pktNode
}

type pktNode struct {
	f    *Packet
	next *pktNode
}

func (g *gop) init() *gop {
	g.next = g
	g.prev = g
	return g
}

func (g *gop) Next() *gop {
	if g.next == nil {
		return g.init()
	}
	return g.next
}

func (g *gop) Prev() *gop {
	if g.next == nil {
		return g.init()
	}
	return g.prev
}

func (g *gop) Move(n int) *gop {
	if g.next == nil {
		return g.init()
	}
	switch {
	case n < 0:
		for ; n < 0; n++ {
			g = g.prev
		}
	case n > 0:
		for ; n > 0; n-- {
			g = g.next
		}
	}
	return g
}

func newGopQueue(n int) *gop {
	if n <= 0 {
		return nil
	}
	g := new(gop)
	p := g
	for i := 1; i < n; i++ {
		p.next = &gop{prev: p}
		p = p.next
	}
	p.next = g
	g.prev = p
	return g
}

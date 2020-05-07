// gop
// a list of frames begin with key frame
package av

// |-----|    |-----|    |-----|    |-----|
// | gop | -> | gop | -> | gop | -> | gop |
// |-----|    |-----|    |-----|    |-----|
//    ↓          ↓          ↓          ↓
//   pkt   |->  pkt   |->  pkt   |->  pkt
//    ↓    |     ↓    |     ↓    |     ↓
//   pkt   |    pkt   |    pkt   |    pkt
//    ↓    |     ↓    |     ↓    |     ↓
//   pkt   |    pkt   |    pkt   |    pkt
//    ↓    |     ↓    |     ↓    |     ↓
//   pkt  -|    pkt  -|    pkt  -|    pkt
// gop
type gop struct {
	// sum of all the frame delta timestamp in this gop
	duration uint32
	// first node timestamp of cache
	timestamp uint32
	// idx of first frame
	idx    int64
	length int

	// codec
	isCodec      bool
	codecVersion int

	next, prev      *gop
	node, nodeStart *pktNode
}

func (g *gop) Write(f *Packet) {
	n := &pktNode{
		f:    f,
		next: nil,
	}
	// TODO: error, g.node should not be nil
	if g.node == nil {
		g.node = n
	} else {
		g.node.next = n
	}
	g.duration += f.Timestamp
	g.length++
}

func (g *gop) WriteInNewGop(f *Packet, idx int64) *gop {
	ng := &gop{
		idx:       idx,
		prev:      g,
		nodeStart: &pktNode{f: f},
	}
	ng.node = ng.nodeStart
	g.next = ng
	g.node.next = ng.nodeStart

	return ng
}

type pktNode struct {
	f    *Packet
	next *pktNode
}

package av

// use Packet instead of interface{}

// =================================================
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ring implements operations on circular lists.

// A packetRing is an element of a circular list, or ring.
// Rings do not have a beginning or end; a pointer to any ring element
// serves as reference to the entire ring. Empty rings are represented
// as nil packetRing pointers. The zero value for a packetRing is a one-element
// ring with a nil Value.
//
type packetRing struct {
	next, prev *packetRing
	Value      *Packet // for use by client; untouched by this library
}

func (r *packetRing) init() *packetRing {
	r.next = r
	r.prev = r
	return r
}

// Next returns the next ring element. r must not be empty.
func (r *packetRing) Next() *packetRing {
	if r.next == nil {
		return r.init()
	}
	return r.next
}

// Prev returns the previous ring element. r must not be empty.
func (r *packetRing) Prev() *packetRing {
	if r.next == nil {
		return r.init()
	}
	return r.prev
}

// New creates a ring of n elements.
func newPacketRing(n int) *packetRing {
	if n <= 0 {
		return nil
	}
	r := new(packetRing)
	p := r
	for i := 1; i < n; i++ {
		p.next = &packetRing{prev: p}
		p = p.next
	}
	p.next = r
	r.prev = p
	return r
}

// Link connects ring r with ring s such that r.Next()
// becomes s and returns the original value for r.Next().
// r must not be empty.
//
// If r and s point to the same ring, linking
// them removes the elements between r and s from the ring.
// The removed elements form a subring and the result is a
// reference to that subring (if no elements were removed,
// the result is still the original value for r.Next(),
// and not nil).
//
// If r and s point to different rings, linking
// them creates a single ring with the elements of s inserted
// after r. The result points to the element following the
// last element of s after insertion.
//
func (r *packetRing) Link(s *packetRing) *packetRing {
	n := r.Next()
	if s != nil {
		p := s.Prev()
		// Note: Cannot use multiple assignment because
		// evaluation order of LHS is not specified.
		r.next = s
		s.prev = r
		n.prev = p
		p.next = n
	}
	return n
}

// Len computes the number of elements in ring r.
// It executes in time proportional to the number of elements.
//
func (r *packetRing) Len() int {
	n := 0
	if r != nil {
		n = 1
		for p := r.Next(); p != r; p = p.next {
			n++
		}
	}
	return n
}

// Do calls function f on each element of the ring, in forward order.
// The behavior of Do is undefined if f changes *r.
func (r *packetRing) Do(f func(*Packet)) {
	if r != nil {
		f(r.Value)
		for p := r.Next(); p != r; p = p.next {
			f(p.Value)
		}
	}
}

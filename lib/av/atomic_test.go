// Is assigning a pointer atomic in golang?
// Is assignint a int64 atomic in golang?
package av

import (
	"sync/atomic"
	"testing"
)

var (
	vInt64      int64
	pInt64      *int64
	valueLoop   bool
	pointerLoop bool
)

func writeValue(value int64) {
	for valueLoop {
		vInt64 = value
	}
}

func Test_WriteValue(t *testing.T) {
	// assign int64
	vInt64 = 1
	valueLoop = true
	go writeValue(1)
	go writeValue(2)

	for i := 0; i < 100000; i++ {
		if vInt64 != 1 && vInt64 != 2 {
			t.Errorf(">> !!!!! assign int64 is not atomic !!!!!")
			return
		}
	}
	valueLoop = false
}

func swapPointer(p *int64) {
	for pointerLoop {
		pInt64 = p
	}
}

func Test_SwapPointer(t *testing.T) {
	// assign pointer
	var pp1 int64 = 1
	var pp2 int64 = 2
	pointerLoop = true
	pInt64 = &pp1

	go swapPointer(&pp1)
	go swapPointer(&pp2)

	for i := 0; i < 100000; i++ {
		if pInt64 != &pp1 && pInt64 != &pp2 {
			t.Errorf(">>> !!!!! assign pointer it not atomic !!!!!")
			return
		}
	}
	pointerLoop = false
}

func Benchmark_ValueAtomic(b *testing.B) {
	var v int64
	for i := 0; i < b.N; i++ {
		atomic.AddInt64(&v, 1)
		atomic.AddInt64(&v, 1)
		atomic.AddInt64(&v, 1)
		atomic.AddInt64(&v, 1)
		atomic.AddInt64(&v, 1)
		atomic.AddInt64(&v, 1)
	}
}

func add(p int64) int64 {
	return p + 1
}

func Benchmark_Value(b *testing.B) {
	var v int64
	for i := 0; i < b.N; i++ {
		v = add(v)
		v = add(v)
		v = add(v)
		v = add(v)
		v = add(v)
		v = add(v)
	}
}

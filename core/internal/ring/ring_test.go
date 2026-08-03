package ring

import (
	"sync"
	"testing"

	"omniproxy/core/api"
)

func TestPushDrain(t *testing.T) {
	r := New(2)
	if got := r.Drain(); len(got) != 0 {
		t.Fatalf("Drain on empty ring returned %d events", len(got))
	}
	r.Push(api.Event{Type: "a"})
	r.Push(api.Event{Type: "b"})
	r.Push(api.Event{Type: "c"})
	got := r.Drain()
	if len(got) != 2 {
		t.Fatalf("Drain returned %d events, want 2", len(got))
	}
	if got[0].Type != "b" || got[1].Type != "c" {
		t.Fatalf("oldest not dropped: %+v", got)
	}
	if len(r.Drain()) != 0 {
		t.Fatalf("ring not empty after Drain")
	}
}

func TestCapDefault(t *testing.T) {
	r := New(0)
	for i := 0; i < 600; i++ {
		r.Push(api.Event{Type: "x"})
	}
	if got := len(r.Drain()); got != 512 {
		t.Fatalf("ring cap = %d, want 512", got)
	}
}

func TestConcurrentPushDrain(t *testing.T) {
	r := New(128)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				r.Push(api.Event{Type: "e"})
			}
		}()
	}
	wg.Wait()
	_ = r.Drain()
}

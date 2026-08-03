package watcher

import (
	"context"
	"testing"
	"time"
)

// staticWatcher emits one snapshot immediately and then blocks until ctx done.
type staticWatcher struct {
	states map[string]State
}

func (s *staticWatcher) Run(ctx context.Context, out chan<- Snapshot) {
	snap := Snapshot{States: s.states, PollTime: time.Now()}
	send(ctx, out, snap)
	<-ctx.Done()
}

func TestMultiWatcherMergesSnapshots(t *testing.T) {
	wa := &staticWatcher{states: map[string]State{"/wt-a": Working}}
	wb := &staticWatcher{states: map[string]State{"/wt-b": Done}}

	m := &MultiWatcher{Watchers: []Watcher{wa, wb}}
	ch := make(chan Snapshot, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go m.Run(ctx, ch)

	seen := map[string]State{}
	deadline := time.After(150 * time.Millisecond)
	for len(seen) < 2 {
		select {
		case snap := <-ch:
			for path, st := range snap.States {
				seen[path] = st
			}
		case <-deadline:
			t.Fatalf("timed out; seen so far: %v", seen)
		}
	}
	if seen["/wt-a"] != Working {
		t.Fatalf("/wt-a = %v, want Working", seen["/wt-a"])
	}
	if seen["/wt-b"] != Done {
		t.Fatalf("/wt-b = %v, want Done", seen["/wt-b"])
	}
}

func TestMultiWatcherEmpty(t *testing.T) {
	m := &MultiWatcher{}
	ch := make(chan Snapshot, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// Should not block or panic with zero watchers.
	go m.Run(ctx, ch)
	<-ctx.Done()
}

package storage

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadBudgetSharedAcrossStoresAndRestart(t *testing.T) {
	// Arrange: independent connections represent independent MCP processes.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	stores := make([]*Store, 2)
	for i := range stores {
		s, err := Open(ctx, path, nil, 90)
		if err != nil {
			t.Fatal(err)
		}
		stores[i] = s
		defer s.Close()
	}
	at := time.Now().UnixNano()
	var allowed atomic.Int32
	var wg sync.WaitGroup
	// Act: simultaneous calls cannot multiply burst by the number of readers.
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := stores[i%2].allowReadAt(ctx, at)
			if err != nil {
				t.Error(err)
			}
			if ok {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	// Assert
	if allowed.Load() != 20 {
		t.Fatalf("shared burst=%d", allowed.Load())
	}
	// Act/Assert: new process preserves exhausted bucket; rollback does not refill.
	reopened, err := Open(ctx, path, nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, stamp := range []int64{at, at - int64(time.Second), at + int64(99*time.Millisecond)} {
		ok, err := reopened.allowReadAt(ctx, stamp)
		if err != nil || ok {
			t.Fatalf("premature refill at %d: %v %v", stamp, ok, err)
		}
	}
	ok, err := reopened.allowReadAt(ctx, at+int64(100*time.Millisecond))
	if err != nil || !ok {
		t.Fatalf("10/s refill: %v %v", ok, err)
	}
	if ok, err = reopened.allowReadAt(ctx, at+int64(100*time.Millisecond)); err != nil || ok {
		t.Fatalf("refill token reused: %v %v", ok, err)
	}
}

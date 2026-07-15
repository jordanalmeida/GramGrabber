package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestChunkCacheDedupsConcurrentFetches(t *testing.T) {
	c := NewChunkCache(10 << 20)
	var calls atomic.Int32
	key := chunkKey{doc: 1, off: 0}

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := c.Get(context.Background(), key, func() ([]byte, error) {
				calls.Add(1)
				return []byte("chunk"), nil
			})
			if err != nil || string(data) != "chunk" {
				t.Errorf("unexpected result: %q %v", data, err)
			}
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("expected 1 fetch, got %d", n)
	}
}

func TestChunkCacheDoesNotCacheErrors(t *testing.T) {
	c := NewChunkCache(10 << 20)
	key := chunkKey{doc: 2, off: 0}
	var calls atomic.Int32

	_, err := c.Get(context.Background(), key, func() ([]byte, error) {
		calls.Add(1)
		return nil, fmt.Errorf("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	data, err := c.Get(context.Background(), key, func() ([]byte, error) {
		calls.Add(1)
		return []byte("ok"), nil
	})
	if err != nil || string(data) != "ok" {
		t.Fatalf("retry failed: %q %v", data, err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("expected 2 fetches (error not cached), got %d", n)
	}
}

func TestChunkCacheEvictsOldest(t *testing.T) {
	c := NewChunkCache(3) // fits 3 one-byte chunks
	ctx := context.Background()
	fetch := func(b byte) func() ([]byte, error) {
		return func() ([]byte, error) { return []byte{b}, nil }
	}
	for i := int64(0); i < 5; i++ {
		if _, err := c.Get(ctx, chunkKey{doc: 3, off: i}, fetch(byte(i))); err != nil {
			t.Fatal(err)
		}
	}
	if c.curBytes > 3 {
		t.Fatalf("cache over budget: %d bytes", c.curBytes)
	}
	if c.Has(chunkKey{doc: 3, off: 0}) {
		t.Fatal("oldest entry should have been evicted")
	}
	if !c.Has(chunkKey{doc: 3, off: 4}) {
		t.Fatal("newest entry should be present")
	}
}

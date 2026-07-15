package core

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/gotd/td/tg"
)

// readAhead is how many chunks are prefetched in parallel past the read
// position. MP4 files keep their index (moov atom) at the end, so players
// burst-read several MB before playback starts — parallel fetches make
// that burst ~5x faster than sequential chunks.
const readAhead = 4

/* ---------- shared chunk cache ---------- */

type chunkKey struct {
	doc int64
	off int64
}

type cacheEntry struct {
	data []byte
	err  error
	done chan struct{}
}

// ChunkCache is a byte-bounded LRU of Telegram file chunks shared across
// stream requests. Browsers probe the same ranges repeatedly (head, moov,
// head again); without this every probe re-downloads from Telegram.
// It also deduplicates in-flight fetches of the same chunk.
type ChunkCache struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	entries  map[chunkKey]*cacheEntry
	order    []chunkKey // oldest first
}

func NewChunkCache(maxBytes int64) *ChunkCache {
	return &ChunkCache{maxBytes: maxBytes, entries: map[chunkKey]*cacheEntry{}}
}

// Get returns the chunk, fetching (once) on miss. Concurrent callers for
// the same key share a single fetch.
func (c *ChunkCache) Get(ctx context.Context, key chunkKey, fetch func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		c.touch(key)
		c.mu.Unlock()
		select {
		case <-e.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if e.err != nil {
			return nil, e.err
		}
		return e.data, nil
	}
	e := &cacheEntry{done: make(chan struct{})}
	c.entries[key] = e
	c.order = append(c.order, key)
	c.mu.Unlock()

	e.data, e.err = fetch()
	close(e.done)

	c.mu.Lock()
	if e.err != nil {
		// Don't cache failures; the next caller retries.
		c.remove(key)
	} else {
		c.curBytes += int64(len(e.data))
		c.evict()
	}
	c.mu.Unlock()
	return e.data, e.err
}

func (c *ChunkCache) Has(key chunkKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[key]
	return ok
}

// touch moves key to the end of the LRU order (must hold mu).
func (c *ChunkCache) touch(key chunkKey) {
	for i, k := range c.order {
		if k == key {
			c.order = append(append(c.order[:i:i], c.order[i+1:]...), key)
			return
		}
	}
}

func (c *ChunkCache) remove(key chunkKey) {
	if e, ok := c.entries[key]; ok {
		c.curBytes -= int64(len(e.data))
		delete(c.entries, key)
	}
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

// evict drops oldest completed entries until under budget (must hold mu).
func (c *ChunkCache) evict() {
	for c.curBytes > c.maxBytes && len(c.order) > 0 {
		evicted := false
		for _, key := range c.order {
			e := c.entries[key]
			select {
			case <-e.done: // completed; safe to drop
				c.remove(key)
				evicted = true
			default: // in flight; skip
				continue
			}
			break
		}
		if !evicted {
			return // everything in flight
		}
	}
}

/* ---------- streaming reader ---------- */

// TGReader is an io.ReadSeeker over a Telegram document, fetching 1MB
// chunks on demand — this is what powers "watch without downloading".
// http.ServeContent drives it, so HTTP Range requests (video seeking)
// work naturally. Reads pull through the shared ChunkCache and prefetch
// `readAhead` chunks in parallel through the connection pool.
type TGReader struct {
	ctx     context.Context
	api     *tg.Client
	size    int64
	docID   int64
	cache   *ChunkCache
	refresh func(ctx context.Context) (*tg.InputDocumentFileLocation, error)

	mu          sync.Mutex
	locMu       sync.Mutex
	loc         *tg.InputDocumentFileLocation
	off         int64
	buf         []byte
	bufOff      int64 // aligned start offset of buf; -1 when empty
	lastAligned int64 // last chunk we spawned prefetches from
}

func NewTGReader(
	ctx context.Context,
	api *tg.Client,
	loc *tg.InputDocumentFileLocation,
	size int64,
	cache *ChunkCache,
	refresh func(ctx context.Context) (*tg.InputDocumentFileLocation, error),
) *TGReader {
	return &TGReader{
		ctx: ctx, api: api, loc: loc, size: size,
		docID: loc.ID, cache: cache, refresh: refresh,
		bufOff: -1, lastAligned: -1,
	}
}

func (r *TGReader) Size() int64 { return r.size }

func (r *TGReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.off + offset
	case io.SeekEnd:
		abs = r.size + offset
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}
	if abs < 0 {
		return 0, fmt.Errorf("negative seek position")
	}
	r.off = abs
	return abs, nil
}

func (r *TGReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.off >= r.size {
		return 0, io.EOF
	}
	aligned := r.off - (r.off % chunkSize)

	// Fire parallel readahead once per chunk boundary.
	if aligned != r.lastAligned {
		r.lastAligned = aligned
		for k := int64(1); k <= readAhead; k++ {
			off := aligned + k*chunkSize
			key := chunkKey{doc: r.docID, off: off}
			if off < r.size && !r.cache.Has(key) {
				go func(off int64) { _, _ = r.getChunk(off) }(off)
			}
		}
	}

	if r.bufOff != aligned {
		data, err := r.getChunk(aligned)
		if err != nil {
			return 0, err
		}
		r.buf, r.bufOff = data, aligned
	}

	start := r.off - r.bufOff
	if start >= int64(len(r.buf)) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[start:])
	r.off += int64(n)
	return n, nil
}

// getChunk pulls one chunk through the cache; safe for concurrent use.
func (r *TGReader) getChunk(off int64) ([]byte, error) {
	key := chunkKey{doc: r.docID, off: off}
	return r.cache.Get(r.ctx, key, func() ([]byte, error) {
		loc := r.currentLoc()
		// fetchChunk (not fetchChunkAt) so dead-connection blips retry.
		data, err := fetchChunk(r.ctx, r.api, loc, off)
		if err != nil && isFileRefError(err) && r.refresh != nil {
			fresh, rerr := r.refresh(r.ctx)
			if rerr != nil {
				return nil, fmt.Errorf("refreshing file reference: %w", rerr)
			}
			r.setLoc(fresh)
			data, err = fetchChunk(r.ctx, r.api, fresh, off)
		}
		return data, err
	})
}

// loc has its own mutex: Read holds r.mu while prefetch goroutines
// (which don't) also need the current location.
func (r *TGReader) currentLoc() *tg.InputDocumentFileLocation {
	r.locMu.Lock()
	defer r.locMu.Unlock()
	return r.loc
}

func (r *TGReader) setLoc(loc *tg.InputDocumentFileLocation) {
	r.locMu.Lock()
	defer r.locMu.Unlock()
	r.loc = loc
}

func fetchChunkAt(ctx context.Context, api *tg.Client, loc *tg.InputDocumentFileLocation, off int64) ([]byte, error) {
	res, err := api.UploadGetFile(ctx, &tg.UploadGetFileRequest{
		Location: loc,
		Offset:   off,
		Limit:    chunkSize,
	})
	if err != nil {
		return nil, err
	}
	switch d := res.(type) {
	case *tg.UploadFile:
		return d.Bytes, nil
	case *tg.UploadFileCDNRedirect:
		return nil, fmt.Errorf("CDN redirect not supported for streaming")
	default:
		return nil, fmt.Errorf("unexpected upload response: %T", res)
	}
}

func isFileRefError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FILE_REFERENCE")
}

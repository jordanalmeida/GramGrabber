package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/tg"
)

// 1MB is Telegram's maximum per request (4KB-aligned): half the round-trips
// of the classic 512KB parts.
const chunkSize = 1024 * 1024

// partState is the resume sidecar (<file>.ggpart): which chunks are done.
type partState struct {
	ChunkSize int    `json:"chunkSize"`
	Size      int64  `json:"size"`
	Done      []bool `json:"done"`
}

func partPath(outPath string) string { return outPath + ".ggpart" }

func loadPartState(outPath string, size int64, chunks int) *partState {
	data, err := os.ReadFile(partPath(outPath))
	if err == nil {
		var st partState
		if json.Unmarshal(data, &st) == nil &&
			st.ChunkSize == chunkSize && st.Size == size && len(st.Done) == chunks {
			return &st
		}
	}
	return &partState{ChunkSize: chunkSize, Size: size, Done: make([]bool, chunks)}
}

func (st *partState) save(outPath string) {
	if data, err := json.Marshal(st); err == nil {
		_ = os.WriteFile(partPath(outPath), data, 0o644)
	}
}

// ParallelDownload fetches a document over `threads` concurrent connections
// with byte-accurate progress and chunk-level resume. Interrupt it at any
// point and a later call picks up exactly where it stopped.
func ParallelDownload(
	ctx context.Context,
	api *tg.Client,
	loc *tg.InputDocumentFileLocation,
	size int64,
	outPath string,
	threads int,
	onProgress func(done int64),
) error {
	if size <= 0 {
		return fmt.Errorf("invalid file size %d", size)
	}
	if threads <= 0 {
		threads = 8
	}
	chunks := int((size + chunkSize - 1) / chunkSize)
	st := loadPartState(outPath, size, chunks)

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		return err
	}

	var done int64
	for i, d := range st.Done {
		if d {
			n := int64(chunkSize)
			if i == chunks-1 {
				n = size - int64(i)*chunkSize
			}
			done += n
		}
	}
	if onProgress != nil {
		onProgress(done)
	}

	var (
		next     int64 // next chunk index to claim
		stMu     sync.Mutex
		errMu    sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// atomic.Value can't hold differently-typed errors; a mutex can.
	fail := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
		cancel()
	}

	worker := func() {
		defer wg.Done()
		for {
			i := int(atomic.AddInt64(&next, 1) - 1)
			if i >= chunks || dlCtx.Err() != nil {
				return
			}
			stMu.Lock()
			skip := st.Done[i]
			stMu.Unlock()
			if skip {
				continue
			}

			offset := int64(i) * chunkSize
			data, err := fetchChunk(dlCtx, api, loc, offset)
			if err != nil {
				fail(err)
				return
			}
			want := int64(chunkSize)
			if i == chunks-1 {
				want = size - offset
			}
			if int64(len(data)) > want {
				data = data[:want]
			}
			if _, err := f.WriteAt(data, offset); err != nil {
				fail(err)
				return
			}

			stMu.Lock()
			st.Done[i] = true
			st.save(outPath)
			stMu.Unlock()

			d := atomic.AddInt64(&done, int64(len(data)))
			if onProgress != nil {
				onProgress(d)
			}
		}
	}

	wg.Add(threads)
	for range threads {
		go worker()
	}
	wg.Wait()

	errMu.Lock()
	err = firstErr
	errMu.Unlock()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = os.Remove(partPath(outPath))
	return nil
}

// fetchChunk requests one block, retrying transient failures. Pool
// connections get killed by Telegram under heavy transfer ("engine was
// closed" / "connection dead") and take a few seconds to re-establish,
// so retries are patient rather than fast.
func fetchChunk(ctx context.Context, api *tg.Client, loc *tg.InputDocumentFileLocation, offset int64) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		data, err := fetchChunkAt(ctx, api, loc, offset)
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if strings.Contains(err.Error(), "CDN redirect") {
			return nil, err // not transient; let the caller fall back
		}
		lastErr = err
	}
	return nil, lastErr
}

// IsPartial reports whether a resume sidecar exists for the file.
func IsPartial(outPath string) bool {
	_, err := os.Stat(partPath(outPath))
	return err == nil
}

// RemovePartState drops the resume sidecar (used when a fallback
// downloader takes over the file).
func RemovePartState(outPath string) {
	_ = os.Remove(partPath(outPath))
}

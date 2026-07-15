package studio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram/downloader"

	"telegram-downloader/internal/core"
)

type JobState string

const (
	JobQueued   JobState = "queued"
	JobRunning  JobState = "running"
	JobDone     JobState = "done"
	JobError    JobState = "error"
	JobCanceled JobState = "canceled"
)

type Job struct {
	ID        int64    `json:"id"`
	Channel   string   `json:"channel"`
	ChannelID int64    `json:"channelId"`
	MsgID     int      `json:"msgId"`
	Name      string   `json:"name"`
	Size      int64    `json:"size"`
	State     JobState `json:"state"`
	Error     string   `json:"error,omitempty"`

	done         atomic.Int64
	speedBytes   atomic.Int64 // bytes at last speed sample
	speed        atomic.Int64 // bytes/sec
	userCanceled atomic.Bool  // user hit Cancel (vs. a dropped connection)
	cancel       context.CancelFunc
}

type jobJSON struct {
	ID      int64    `json:"id"`
	Channel string   `json:"channel"`
	MsgID   int      `json:"msgId"`
	Name    string   `json:"name"`
	Size    int64    `json:"size"`
	State   JobState `json:"state"`
	Error   string   `json:"error,omitempty"`
	Done    int64    `json:"done"`
	Speed   int64    `json:"speed"`
}

// JobManager runs a few files concurrently (8 threads inside each file).
// Telegram caps speed per file server-side, so 2-3 simultaneous files
// raise aggregate throughput when emptying a channel.
type JobManager struct {
	engine *Engine
	mu     sync.Mutex
	nextID int64
	jobs   []*Job
	queue  chan *Job
}

func newJobManager(e *Engine, workers int) *JobManager {
	m := &JobManager{engine: e, queue: make(chan *Job, 256)}
	if workers < 1 {
		workers = 1
	}
	for range workers {
		go m.worker()
	}
	go m.speedometer()
	return m
}

func (m *JobManager) Enqueue(ch core.ChannelInfo, videos []core.VideoInfo) []*Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	var added []*Job
	for _, v := range videos {
		if m.hasActive(ch.ID, v.MsgID) {
			continue
		}
		m.nextID++
		job := &Job{
			ID:        m.nextID,
			Channel:   ch.Title,
			ChannelID: ch.ID,
			MsgID:     v.MsgID,
			Name:      v.Name,
			Size:      v.Size,
			State:     JobQueued,
		}
		m.jobs = append(m.jobs, job)
		added = append(added, job)
		select {
		case m.queue <- job:
		default:
			job.State = JobError
			job.Error = "queue full"
		}
	}
	return added
}

func (m *JobManager) hasActive(channelID int64, msgID int) bool {
	for _, j := range m.jobs {
		if j.ChannelID == channelID && j.MsgID == msgID &&
			(j.State == JobQueued || j.State == JobRunning) {
			return true
		}
	}
	return false
}

func (m *JobManager) Cancel(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID != id {
			continue
		}
		switch j.State {
		case JobQueued:
			j.userCanceled.Store(true)
			j.State = JobCanceled
		case JobRunning:
			j.userCanceled.Store(true)
			if j.cancel != nil {
				j.cancel()
			}
		}
		return true
	}
	return false
}

// ClearFinished drops completed/failed/canceled jobs from the list.
func (m *JobManager) ClearFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		if j.State == JobQueued || j.State == JobRunning {
			kept = append(kept, j)
		}
	}
	m.jobs = kept
}

func (m *JobManager) List() []jobJSON {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]jobJSON, 0, len(m.jobs))
	for i := len(m.jobs) - 1; i >= 0; i-- { // newest first
		j := m.jobs[i]
		out = append(out, jobJSON{
			ID: j.ID, Channel: j.Channel, MsgID: j.MsgID, Name: j.Name,
			Size: j.Size, State: j.State, Error: j.Error,
			Done: j.done.Load(), Speed: j.speed.Load(),
		})
	}
	return out
}

// ActiveDownloadState reports downloaded/partial status for the videos view.
func (m *JobManager) worker() {
	for job := range m.queue {
		m.mu.Lock()
		if job.State != JobQueued {
			m.mu.Unlock()
			continue
		}
		job.State = JobRunning
		m.mu.Unlock()

		// Whole-job retry: connection blips ("engine was closed", the
		// client reconnecting) kill a single attempt, not the job. The
		// .ggpart sidecar makes re-attempts resume, not restart.
		var err error
		for attempt := 1; ; attempt++ {
			err = m.run(job)
			if err == nil || job.userCanceled.Load() ||
				attempt >= 3 || !isTransient(err) {
				break
			}
			time.Sleep(time.Duration(attempt) * 3 * time.Second)
		}

		m.mu.Lock()
		switch {
		case err == nil:
			job.State = JobDone
			job.done.Store(job.Size)
		case job.userCanceled.Load() || job.State == JobCanceled:
			job.State = JobCanceled
		default:
			job.State = JobError
			job.Error = err.Error()
		}
		job.cancel = nil
		m.mu.Unlock()
	}
}

// isTransient reports errors worth retrying: transport drops and the
// context cancellations they cause (user cancels are filtered earlier).
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	s := err.Error()
	for _, marker := range []string{
		"engine was closed", "engine forcibly closed",
		"connection dead", "retryUntilAck", "not connected",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func (m *JobManager) run(job *Job) error {
	ch, ok := m.engine.Channel(job.ChannelID)
	if !ok {
		return fmt.Errorf("channel not loaded; open it again in Channels")
	}
	api, runCtx, err := m.engine.LongAPI()
	if err != nil {
		return err
	}
	// Heavy transfer goes through the connection pool when available.
	mediaAPI, _, err := m.engine.MediaAPI()
	if err != nil {
		mediaAPI = api
	}
	ctx, cancel := context.WithCancel(runCtx)
	m.mu.Lock()
	job.cancel = cancel
	m.mu.Unlock()
	defer cancel()

	// Fresh document: file references expire quickly.
	fetchCtx, fetchCancel := context.WithTimeout(ctx, 60*time.Second)
	doc, v, err := core.FetchDocument(fetchCtx, api, ch, job.MsgID)
	fetchCancel()
	if err != nil {
		return err
	}
	job.Size = v.Size

	dir := filepath.Join(m.engine.Config().DownloadsDir, ch.FolderName())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(dir, v.Name)

	// Already complete?
	if info, err := os.Stat(outPath); err == nil &&
		info.Size() == v.Size && !core.IsPartial(outPath) {
		return nil
	}

	loc := doc.AsInputDocumentFileLocation()
	progress := func(done int64) { job.done.Store(done) }

	err = core.ParallelDownload(ctx, mediaAPI, loc, v.Size, outPath, 8, progress)
	if err == nil || ctx.Err() != nil {
		return err
	}

	// The pool's connections can be killed under heavy transfer; retry
	// over the main client connection (independent transport, and the
	// .ggpart sidecar means only missing chunks are refetched).
	if mediaAPI != api {
		err = core.ParallelDownload(ctx, api, loc, v.Size, outPath, 4, progress)
		if err == nil || ctx.Err() != nil {
			return err
		}
	}

	// Last resort: gotd's native downloader handles cases ours doesn't
	// (CDN redirects, DC quirks). Progress becomes approximate.
	core.RemovePartState(outPath)
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if info, err := os.Stat(outPath); err == nil {
					job.done.Store(min(info.Size(), v.Size))
				}
			}
		}
	}()
	_, fbErr := downloader.NewDownloader().
		Download(api, loc).
		WithThreads(4).
		ToPath(ctx, outPath)
	close(stop)
	if fbErr != nil {
		return fmt.Errorf("%v (fallback: %v)", err, fbErr)
	}
	return nil
}

// speedometer samples running jobs once per second.
func (m *JobManager) speedometer() {
	for range time.Tick(time.Second) {
		m.mu.Lock()
		for _, j := range m.jobs {
			if j.State == JobRunning {
				d := j.done.Load()
				j.speed.Store(d - j.speedBytes.Load())
				j.speedBytes.Store(d)
			} else {
				j.speed.Store(0)
			}
		}
		m.mu.Unlock()
	}
}

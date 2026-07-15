package studio

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	"telegram-downloader/internal/core"
)

type Phase string

const (
	PhaseUnconfigured Phase = "unconfigured"
	PhaseConnecting   Phase = "connecting"
	PhaseNeedPhone    Phase = "need_phone"
	PhaseNeedCode     Phase = "need_code"
	PhaseNeedPassword Phase = "need_password"
	PhaseReady        Phase = "ready"
	PhaseError        Phase = "error"
)

// Engine owns the Telegram client lifecycle. The gotd client runs inside
// a goroutine (client.Run blocks); HTTP handlers reach Telegram through
// API(), which is only valid while the phase is ready.
type Engine struct {
	mu      sync.Mutex
	cfg     core.StudioConfig
	phase   Phase
	lastErr string
	user    string

	client  *telegram.Client
	api     *tg.Client
	poolAPI *tg.Client // multi-connection invoker for media transfer
	runCtx  context.Context
	cancel  context.CancelFunc
	auth    *webAuth

	channels map[int64]core.ChannelInfo // cache for download jobs

	Jobs *JobManager
	// StreamCache keeps recently streamed chunks in RAM so browser range
	// probes don't re-download the same bytes from Telegram.
	StreamCache *core.ChunkCache
}

func NewEngine() (*Engine, error) {
	cfg, err := core.LoadStudioConfig()
	if err != nil {
		return nil, err
	}
	e := &Engine{
		cfg:         cfg,
		phase:       PhaseUnconfigured,
		channels:    map[int64]core.ChannelInfo{},
		StreamCache: core.NewChunkCache(128 << 20), // 128MB
	}
	e.Jobs = newJobManager(e, cfg.ParallelDownloads)
	if cfg.Configured() {
		e.Start()
	}
	return e, nil
}

func (e *Engine) sessionPath() string {
	dir, err := core.ConfigDir()
	if err != nil {
		return "studio-session.json"
	}
	return filepath.Join(dir, "studio-session.json")
}

func (e *Engine) Config() core.StudioConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *Engine) SetConfig(cfg core.StudioConfig) error {
	if err := cfg.Save(); err != nil {
		return err
	}
	e.mu.Lock()
	credsChanged := e.cfg.AppID != cfg.AppID || e.cfg.AppHash != cfg.AppHash
	notRunning := e.cancel == nil
	e.cfg = cfg
	e.mu.Unlock()
	if credsChanged || notRunning {
		e.Start()
	}
	return nil
}

type State struct {
	Configured   bool   `json:"configured"`
	Phase        Phase  `json:"phase"`
	Error        string `json:"error,omitempty"`
	User         string `json:"user,omitempty"`
	DownloadsDir string `json:"downloadsDir"`
	AppID        int    `json:"appId"`
}

func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return State{
		Configured:   e.cfg.Configured(),
		Phase:        e.phase,
		Error:        e.lastErr,
		User:         e.user,
		DownloadsDir: e.cfg.DownloadsDir,
		AppID:        e.cfg.AppID,
	}
}

func (e *Engine) setPhase(p Phase) {
	e.mu.Lock()
	e.phase = p
	if p != PhaseError {
		e.lastErr = ""
	}
	e.mu.Unlock()
}

// Start (re)connects the Telegram client with the current credentials.
func (e *Engine) Start() {
	e.Stop()

	e.mu.Lock()
	cfg := e.cfg
	if !cfg.Configured() {
		e.phase = PhaseUnconfigured
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	wa := newWebAuth(e)
	// FLOOD_WAIT handling: Telegram answers bursts (channel scans, chunk
	// storms) with 420 + a wait time; the middleware sleeps and retries
	// instead of surfacing the error. Capped so the UI never hangs long.
	waiter := floodwait.NewSimpleWaiter().WithMaxWait(60 * time.Second)
	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: e.sessionPath()},
		Middlewares:    []telegram.Middleware{waiter},
	})
	e.cancel = cancel
	e.client = client
	e.auth = wa
	e.phase = PhaseConnecting
	e.lastErr = ""
	e.mu.Unlock()

	go func() {
		err := client.Run(ctx, func(runCtx context.Context) error {
			flow := auth.NewFlow(wa, auth.SendCodeOptions{})
			if err := client.Auth().IfNecessary(runCtx, flow); err != nil {
				return fmt.Errorf("auth: %w", err)
			}
			self, err := client.Self(runCtx)
			if err != nil {
				return fmt.Errorf("failed to get self: %w", err)
			}
			name := self.FirstName
			if self.Username != "" {
				name = "@" + self.Username
			}

			// Media pool: Telegram caps throughput per TCP connection, so
			// downloads/streaming go through a multi-connection invoker.
			var poolAPI *tg.Client
			var poolClose func() error
			// 4 connections: official clients use ~4 for media; more
			// invites Telegram to kill connections mid-transfer.
			if inv, err := client.Pool(4); err == nil {
				// The pool bypasses client middlewares; re-wrap it so
				// FLOOD_WAIT retries also cover media transfers.
				poolAPI = tg.NewClient(waiter.Handle(inv))
				poolClose = inv.Close
			}

			e.mu.Lock()
			e.api = client.API()
			e.poolAPI = poolAPI
			e.runCtx = runCtx
			e.user = name
			e.phase = PhaseReady
			e.mu.Unlock()

			<-runCtx.Done()
			if poolClose != nil {
				_ = poolClose()
			}
			return nil
		})

		e.mu.Lock()
		e.api = nil
		e.poolAPI = nil
		e.runCtx = nil
		if ctx.Err() == nil { // died on its own (auth failure, network)
			e.phase = PhaseError
			if err != nil {
				e.lastErr = err.Error()
			} else {
				e.lastErr = "connection closed"
			}
		}
		e.mu.Unlock()
	}()
}

func (e *Engine) Stop() {
	e.mu.Lock()
	cancel := e.cancel
	e.cancel = nil
	e.mu.Unlock()
	if cancel != nil {
		cancel()
		// Give the run goroutine a moment to unwind before a restart
		// reuses the session file.
		time.Sleep(150 * time.Millisecond)
	}
}

// API returns a Telegram client bound to a timeout context.
func (e *Engine) API() (*tg.Client, context.Context, context.CancelFunc, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.phase != PhaseReady || e.api == nil || e.runCtx == nil {
		return nil, nil, nil, fmt.Errorf("not connected to Telegram")
	}
	ctx, cancel := context.WithTimeout(e.runCtx, 90*time.Second)
	return e.api, ctx, cancel, nil
}

// LongAPI is like API but without a timeout (downloads); the returned
// context still dies if the client stops.
func (e *Engine) LongAPI() (*tg.Client, context.Context, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.phase != PhaseReady || e.api == nil || e.runCtx == nil {
		return nil, nil, fmt.Errorf("not connected to Telegram")
	}
	return e.api, e.runCtx, nil
}

// MediaAPI returns the multi-connection invoker for heavy transfers
// (downloads, streaming), falling back to the primary connection.
func (e *Engine) MediaAPI() (*tg.Client, context.Context, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.phase != PhaseReady || e.runCtx == nil {
		return nil, nil, fmt.Errorf("not connected to Telegram")
	}
	if e.poolAPI != nil {
		return e.poolAPI, e.runCtx, nil
	}
	if e.api == nil {
		return nil, nil, fmt.Errorf("not connected to Telegram")
	}
	return e.api, e.runCtx, nil
}

func (e *Engine) RememberChannels(chs []core.ChannelInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ch := range chs {
		e.channels[ch.ID] = ch
	}
}

func (e *Engine) Channel(id int64) (core.ChannelInfo, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch, ok := e.channels[id]
	return ch, ok
}

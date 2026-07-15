package studio

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"telegram-downloader/internal/core"
)

//go:embed web
var webFS embed.FS

func NewServer(e *Engine) http.Handler {
	mux := http.NewServeMux()

	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(static))

	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, e.State())
	})

	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		cfg := e.Config()
		writeJSON(w, map[string]any{
			"appId":             cfg.AppID,
			"appHash":           cfg.AppHash,
			"downloadsDir":      cfg.DownloadsDir,
			"parallelDownloads": cfg.ParallelDownloads,
		})
	})

	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AppID             json.Number `json:"appId"`
			AppHash           string      `json:"appHash"`
			DownloadsDir      string      `json:"downloadsDir"`
			ParallelDownloads int         `json:"parallelDownloads"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpError(w, 400, "invalid JSON: %v", err)
			return
		}
		id, err := strconv.Atoi(strings.TrimSpace(body.AppID.String()))
		if err != nil || id <= 0 {
			httpError(w, 400, "App ID must be a positive number")
			return
		}
		hash := strings.TrimSpace(body.AppHash)
		if len(hash) < 16 {
			httpError(w, 400, "App Hash looks too short")
			return
		}
		cfg := e.Config()
		cfg.AppID = id
		cfg.AppHash = hash
		if dir := strings.TrimSpace(body.DownloadsDir); dir != "" {
			cfg.DownloadsDir = dir
		}
		if body.ParallelDownloads != 0 {
			cfg.ParallelDownloads = max(1, min(4, body.ParallelDownloads))
		}
		if err := e.SetConfig(cfg); err != nil {
			httpError(w, 500, "failed to save settings: %v", err)
			return
		}
		writeJSON(w, e.State())
	})

	mux.HandleFunc("POST /api/auth/{step}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpError(w, 400, "invalid JSON: %v", err)
			return
		}
		value := strings.TrimSpace(body.Value)
		if value == "" {
			httpError(w, 400, "value is required")
			return
		}
		if err := e.SubmitAuth(r.PathValue("step"), value); err != nil {
			httpError(w, 409, "%v", err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	// Lets the .app be quit from the interface (the bundle runs headless).
	mux.HandleFunc("POST /api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
		go func() {
			time.Sleep(300 * time.Millisecond)
			e.Stop()
			os.Exit(0)
		}()
	})

	mux.HandleFunc("POST /api/retry", func(w http.ResponseWriter, r *http.Request) {
		e.Start()
		writeJSON(w, e.State())
	})

	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		e.Stop()
		_ = os.Remove(e.sessionPath())
		e.Start()
		writeJSON(w, e.State())
	})

	mux.HandleFunc("GET /api/channels", func(w http.ResponseWriter, r *http.Request) {
		api, ctx, cancel, err := e.API()
		if err != nil {
			httpError(w, 503, "%v", err)
			return
		}
		defer cancel()
		chs, err := core.FetchChannels(ctx, api)
		if err != nil {
			httpError(w, 502, "%v", err)
			return
		}
		e.RememberChannels(chs)
		writeJSON(w, chs)
	})

	mux.HandleFunc("GET /api/videos", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.URL.Query().Get("channel"), 10, 64)
		if err != nil {
			httpError(w, 400, "invalid channel id")
			return
		}
		ch, ok := e.Channel(id)
		if !ok {
			httpError(w, 404, "channel not loaded; refresh Channels first")
			return
		}
		max, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		api, ctx, cancel, err := e.API()
		if err != nil {
			httpError(w, 503, "%v", err)
			return
		}
		defer cancel()
		videos, err := core.ListVideos(ctx, api, ch, max)
		if err != nil {
			httpError(w, 502, "%v", err)
			return
		}

		type videoJSON struct {
			core.VideoInfo
			Status    string `json:"status"`              // none | partial | done
			MediaPath string `json:"mediaPath,omitempty"` // local playback when downloaded
		}
		dir := filepath.Join(e.Config().DownloadsDir, ch.FolderName())
		out := make([]videoJSON, 0, len(videos))
		for _, v := range videos {
			vj := videoJSON{VideoInfo: v, Status: "none"}
			p := filepath.Join(dir, v.Name)
			if info, err := os.Stat(p); err == nil {
				if core.IsPartial(p) {
					vj.Status = "partial"
				} else if info.Size() == v.Size {
					vj.Status = "done"
					vj.MediaPath = "/media/" + filepath.ToSlash(filepath.Join(ch.FolderName(), v.Name))
				}
			}
			out = append(out, vj)
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("POST /api/download", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChannelID int64            `json:"channelId"`
			Videos    []core.VideoInfo `json:"videos"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpError(w, 400, "invalid JSON: %v", err)
			return
		}
		ch, ok := e.Channel(body.ChannelID)
		if !ok {
			httpError(w, 404, "channel not loaded")
			return
		}
		if len(body.Videos) == 0 {
			httpError(w, 400, "no videos selected")
			return
		}
		added := e.Jobs.Enqueue(ch, body.Videos)
		writeJSON(w, map[string]int{"queued": len(added)})
	})

	mux.HandleFunc("GET /api/downloads", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, e.Jobs.List())
	})

	mux.HandleFunc("POST /api/downloads/clear", func(w http.ResponseWriter, r *http.Request) {
		e.Jobs.ClearFinished()
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /api/downloads/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || !e.Jobs.Cancel(id) {
			httpError(w, 404, "job not found")
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("GET /api/library", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, scanLibrary(e.Config().DownloadsDir))
	})

	// Watch without downloading: stream the document straight from
	// Telegram. http.ServeContent + TGReader give us Range support,
	// so seeking in the player works.
	mux.HandleFunc("GET /stream/{channel}/{msg}", func(w http.ResponseWriter, r *http.Request) {
		chID, err1 := strconv.ParseInt(r.PathValue("channel"), 10, 64)
		msgID, err2 := strconv.Atoi(r.PathValue("msg"))
		if err1 != nil || err2 != nil {
			httpError(w, 400, "invalid stream path")
			return
		}
		ch, ok := e.Channel(chID)
		if !ok {
			httpError(w, 404, "channel not loaded; refresh Channels first")
			return
		}
		api, ctx, cancel, err := e.API()
		if err != nil {
			httpError(w, 503, "%v", err)
			return
		}
		doc, v, err := core.FetchDocument(ctx, api, ch, msgID)
		cancel()
		if err != nil {
			httpError(w, 502, "%v", err)
			return
		}

		mediaAPI, runCtx, err := e.MediaAPI()
		if err != nil {
			httpError(w, 503, "%v", err)
			return
		}
		// Tie the Telegram fetches to both the request and the client run.
		streamCtx, stop := context.WithCancel(runCtx)
		defer stop()
		go func() {
			<-r.Context().Done()
			stop()
		}()

		refresh := func(ctx context.Context) (*tg.InputDocumentFileLocation, error) {
			d, _, err := core.FetchDocument(ctx, api, ch, msgID)
			if err != nil {
				return nil, err
			}
			return d.AsInputDocumentFileLocation(), nil
		}
		rd := core.NewTGReader(streamCtx, mediaAPI, doc.AsInputDocumentFileLocation(), v.Size, e.StreamCache, refresh)

		if doc.MimeType != "" {
			w.Header().Set("Content-Type", doc.MimeType)
		}
		http.ServeContent(w, r, v.Name, time.Unix(int64(v.Date), 0), rd)
	})

	mux.HandleFunc("GET /media/", func(w http.ResponseWriter, r *http.Request) {
		root := e.Config().DownloadsDir
		rel := strings.TrimPrefix(r.URL.Path, "/media/")
		rel = filepath.FromSlash(rel)
		full := filepath.Join(root, rel)
		// Path traversal guard: the resolved path must stay inside root.
		relCheck, err := filepath.Rel(root, full)
		if err != nil || strings.HasPrefix(relCheck, "..") {
			httpError(w, 403, "forbidden")
			return
		}
		http.ServeFile(w, r, full) // Range requests supported: seeking works
	})

	return mux
}

type libraryVideo struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // URL path under /media/
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Partial bool   `json:"partial"`
}

type libraryGroup struct {
	Channel string         `json:"channel"`
	Videos  []libraryVideo `json:"videos"`
}

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".webm": true, ".mov": true,
	".avi": true, ".m4v": true, ".ts": true,
}

func scanLibrary(root string) []libraryGroup {
	groups := []libraryGroup{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return groups
	}

	appendVideos := func(dir, channel string) {
		files, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		var vids []libraryVideo
		for _, f := range files {
			if f.IsDir() || !videoExts[strings.ToLower(filepath.Ext(f.Name()))] {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			full := filepath.Join(dir, f.Name())
			rel, _ := filepath.Rel(root, full)
			vids = append(vids, libraryVideo{
				Name:    f.Name(),
				Path:    "/media/" + filepath.ToSlash(rel),
				Size:    info.Size(),
				ModTime: info.ModTime().Unix(),
				Partial: core.IsPartial(full),
			})
		}
		if len(vids) > 0 {
			// Natural name order (M1A2 before M1A10): course content
			// reads in sequence, not in download order.
			sort.Slice(vids, func(i, j int) bool { return naturalLess(vids[i].Name, vids[j].Name) })
			groups = append(groups, libraryGroup{Channel: channel, Videos: vids})
		}
	}

	appendVideos(root, "") // loose files in the root
	for _, entry := range entries {
		if entry.IsDir() {
			appendVideos(filepath.Join(root, entry.Name()), entry.Name())
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Channel < groups[j].Channel })
	return groups
}

// naturalLess compares case-insensitively, treating digit runs as numbers
// so "M1A2" sorts before "M1A10".
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ac, bc := a[ai], b[bi]
		if isDigit(ac) && isDigit(bc) {
			aj := ai
			for aj < len(a) && isDigit(a[aj]) {
				aj++
			}
			bj := bi
			for bj < len(b) && isDigit(b[bj]) {
				bj++
			}
			av, _ := strconv.Atoi(a[ai:aj])
			bv, _ := strconv.Atoi(b[bi:bj])
			if av != bv {
				return av < bv
			}
			ai, bi = aj, bj
			continue
		}
		la, lb := lowerByte(ac), lowerByte(bc)
		if la != lb {
			return la < lb
		}
		ai++
		bi++
	}
	return len(a)-ai < len(b)-bi
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf(format, args...)})
}

package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// StudioConfig is the persisted configuration for the GUI app.
// Unlike the CLI (which reads .env), the studio stores credentials
// entered in the interface at ~/.gramgrabber/config.json.
type StudioConfig struct {
	AppID             int    `json:"app_id"`
	AppHash           string `json:"app_hash"`
	DownloadsDir      string `json:"downloads_dir"`
	ParallelDownloads int    `json:"parallel_downloads"` // simultaneous files (1-4)
}

func clampParallel(n int) int {
	if n < 1 {
		return 2 // default: Telegram also caps per-file speed, so 2 files > 1
	}
	if n > 4 {
		return 4
	}
	return n
}

func (c StudioConfig) Configured() bool {
	return c.AppID != 0 && c.AppHash != ""
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gramgrabber")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func configPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func defaultDownloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "downloads"
	}
	return filepath.Join(home, "GramGrabber")
}

// LoadStudioConfig reads the saved config. On first run it falls back to
// APP_ID/APP_HASH from the environment or a local .env, so existing CLI
// users see their credentials pre-filled.
func LoadStudioConfig() (StudioConfig, error) {
	cfg := StudioConfig{DownloadsDir: defaultDownloadsDir()}

	path, err := configPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
		if cfg.DownloadsDir == "" {
			cfg.DownloadsDir = defaultDownloadsDir()
		}
		cfg.ParallelDownloads = clampParallel(cfg.ParallelDownloads)
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return cfg, err
	}

	// First run: try env / .env as a convenience.
	_ = godotenv.Load()
	if idStr := os.Getenv("APP_ID"); idStr != "" {
		if id, err := strconv.Atoi(idStr); err == nil {
			cfg.AppID = id
		}
	}
	cfg.AppHash = os.Getenv("APP_HASH")
	cfg.ParallelDownloads = clampParallel(cfg.ParallelDownloads)
	return cfg, nil
}

func (c StudioConfig) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

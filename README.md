# GramGrabber

Download videos from Telegram channels — fast, resumable, open source.

**Website:** https://jordanalmeida.github.io/GramGrabber/ ([Português](https://jordanalmeida.github.io/GramGrabber/pt/)) — features, step-by-step tutorial and FAQ.

GramGrabber ships as **two binaries** built from the same core:

| Binary | What it is |
|---|---|
| `gram-grabber` | The classic terminal (CLI) version |
| `gram-grabber-studio` | The visual version: browse channels, download with live progress and **watch your videos in a built-in player** |

## GramGrabber Studio (visual)

A local web app served by a single Go binary — it opens in your browser, nothing leaves your machine.

- Enter your Telegram **API credentials right in the interface** (no `.env` needed); they're stored in `~/.gramgrabber/config.json`
- Sign in to Telegram from the browser (phone → code → 2FA)
- Browse every channel you follow, see each video's size/duration/status, and queue downloads
- **Watch without downloading**: hit ▶ on any video and it streams straight from Telegram, with seeking — no file saved
- Fast transfers: **multi-connection pool** to Telegram, 1MB parts, 8 threads per file, and 1–4 simultaneous files (configurable)
- Byte-accurate progress, live speed and **chunk-level resume** (interrupt anytime; it continues exactly where it stopped)
- **Library with built-in player**: thumbnails, playlists per channel, prev/next, auto-play next
- Videos are organized per channel in `~/GramGrabber/` (configurable)

```bash
go build -o gram-grabber-studio ./cmd/studio
./gram-grabber-studio          # opens http://127.0.0.1:8410
```

Flags: `--port 8410` (0 picks a free port), `--no-open` (don't launch the browser).

## CLI version

### Requirements

- Go 1.25+
- Telegram App ID and Hash (from [my.telegram.org](https://my.telegram.org) → API development tools)

### Setup

1. Copy `env.example` to `.env`
2. Fill in your `APP_ID` and `APP_HASH` in `.env`
3. Run:

```bash
go mod tidy
go build -o gram-grabber .
```

### Usage

```bash
./gram-grabber
```

Follow the prompts to login and select a channel. Videos will be downloaded to the `downloads` directory.

## Features

- 🚀 **Fast Downloads**: 8 parallel threads for maximum speed.
- ⏯️ **Resumable**: interrupted downloads continue from where they stopped.
- 📱 **Interactive**: easy channel selection (menu in the CLI, full UI in Studio).
- 🎬 **Built-in player** (Studio): watch what you downloaded without leaving the app.
- 🔒 **Private**: talks directly to Telegram via MTProto with your own credentials — no third-party servers.

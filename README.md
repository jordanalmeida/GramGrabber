# GramGrabber

A CLI tool written in Go to download videos from Telegram channels.

## Requirements

- Go 1.25+
- Telegram App ID and Hash

## Setup

1. Copy `.env.example` to `.env`
2. Fill in your `APP_ID` and `APP_HASH` in `.env`
3. Run:

```bash
go mod tidy
go build -o gram-grabber .
```

## Usage

```bash
./gram-grabber
```

Follow the prompts to login and select a channel. Videos will be downloaded to the `downloads` directory.

# GramGrabber

A CLI tool written in Go to download videos from Telegram channels.

## Features

- 🚀 **Fast Downloads**: Uses 8 parallel threads for maximum speed.
- ⏯️ **Resumable**: Detects interrupted downloads and lets you resume from where you left off.
- 📱 **Interactive**: Easy channel selection menu.

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

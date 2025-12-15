package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

// DownloadVideos fetches messages from the channel and downloads videos.
func DownloadVideos(ctx context.Context, api *tg.Client, dl *downloader.Downloader, channel *ChannelInfo) error {
	fmt.Printf("Scanning messages in '%s'...\n", channel.Title)

	inputPeer := &tg.InputPeerChannel{
		ChannelID:  channel.ID,
		AccessHash: channel.AccessHash,
	}

	req := &tg.MessagesGetHistoryRequest{
		Peer:  inputPeer,
		Limit: 50,
	}

	history, err := api.MessagesGetHistory(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	messagesSlice, ok := history.(*tg.MessagesChannelMessages)
	if !ok {
		if s, ok := history.(*tg.MessagesMessagesSlice); ok {
			return processMessages(ctx, api, dl, s.Messages)
		}
		if m, ok := history.(*tg.MessagesMessages); ok {
			return processMessages(ctx, api, dl, m.Messages)
		}
		return fmt.Errorf("unexpected history type: %T", history)
	}

	return processMessages(ctx, api, dl, messagesSlice.Messages)
}

func processMessages(ctx context.Context, api *tg.Client, dl *downloader.Downloader, messages []tg.MessageClass) error {
	outputDir := "downloads"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	count := 0
	for _, msg := range messages {
		m, ok := msg.(*tg.Message)
		if !ok {
			continue
		}

		media, ok := m.Media.(*tg.MessageMediaDocument)
		if !ok {
			continue
		}

		doc, ok := media.Document.(*tg.Document)
		if !ok {
			continue
		}

		isVideo := false
		for _, attr := range doc.Attributes {
			if _, ok := attr.(*tg.DocumentAttributeVideo); ok {
				isVideo = true
				break
			}
		}

		if !isVideo {
			continue
		}

		count++
		fileName := fmt.Sprintf("%d_%d.mp4", m.ID, doc.ID)
		for _, attr := range doc.Attributes {
			if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
				fileName = fn.FileName
			}
		}

		outPath := filepath.Join(outputDir, fileName)
		fmt.Printf("[%d] Found video: %s (Size: %d bytes)\n", count, fileName, doc.Size)

		if _, err := os.Stat(outPath); err == nil {
			fmt.Println("   - Already downloaded, skipping.")
			continue
		}

		fmt.Println("   - Downloading...")
		loc := doc.AsInputDocumentFileLocation()

		_, err := dl.Download(api, loc).ToPath(ctx, outPath)
		if err != nil {
			fmt.Printf("   - Failed to download: %v\n", err)
			continue
		}
		fmt.Println("   - Complete!")
	}

	if count == 0 {
		fmt.Println("No videos found in the last batch of messages.")
	} else {
		fmt.Printf("Processed %d videos.\n", count)
	}

	return nil
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		// Check if exists
		if info, err := os.Stat(outPath); err == nil {
			fmt.Printf("   - File exists (Size: %d bytes). [c]ontinue, [n]ew, [s]kip? ", info.Size())
			var choice string
			fmt.Scanln(&choice)
			choice = strings.ToLower(choice)

			if choice == "s" || choice == "skip" {
				fmt.Println("   - Skipped.")
				continue
			}

			if choice == "c" || choice == "continue" {
				if info.Size() >= int64(doc.Size) {
					fmt.Println("   - File already complete. Skipped.")
					continue
				}
				fmt.Printf("   - Resuming from %d bytes...\n", info.Size())
				if err := resumeDownload(ctx, api, doc, outPath, info.Size()); err != nil {
					fmt.Printf("   - Resume failed: %v. Retrying new download.\n", err)
					// Fallthrough to new download
				} else {
					fmt.Println("   - Resume Complete!")
					continue
				}
			}

			// If "n" or new, or resume failed
			if choice == "n" || choice == "new" {
				os.Remove(outPath)
			}
		}

		fmt.Println("   - Downloading...")
		loc := doc.AsInputDocumentFileLocation()

		_, err := dl.Download(api, loc).WithThreads(8).ToPath(ctx, outPath)
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

func resumeDownload(ctx context.Context, api *tg.Client, doc *tg.Document, path string, offset int64) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	chunkSize := 512 * 1024 // 512KB
	loc := doc.AsInputDocumentFileLocation()

	// Convert InputDocumentFileLocation to InputFileLocationClass
	// Actually UploadGetFile expects InputFileLocationClass.
	// InputDocumentFileLocation implements it.

	total := int64(doc.Size)

	for offset < total {
		limit := int(total - offset)
		if limit > chunkSize {
			limit = chunkSize
		}

		// Ensure limit is valid for Telegram (must be divisible by 4KB usually, max 1MB)
		// 512KB is safe.

		req := &tg.UploadGetFileRequest{
			Location: loc,
			Offset:   offset,
			Limit:    limit,
		}

		res, err := api.UploadGetFile(ctx, req)
		if err != nil {
			return err
		}

		switch d := res.(type) {
		case *tg.UploadFile:
			if _, err := f.Write(d.Bytes); err != nil {
				return err
			}
			offset += int64(len(d.Bytes))
			fmt.Printf("\r   - Progress: %.2f%%", float64(offset)/float64(total)*100)
		case *tg.UploadFileCDNRedirect:
			return fmt.Errorf("CDN redirect not supported in resume mode")
		}
	}
	fmt.Println()
	return nil
}

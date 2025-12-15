package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/downloader"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v\n\nPlease create a .env file with:\nAPP_ID=your_id\nAPP_HASH=your_hash\n", err)
	}

	sessionDir := "."
	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(sessionDir, "session.json"),
	}

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: sessionStorage,
	})

	flow := auth.NewFlow(
		termAuth{},
		auth.SendCodeOptions{},
	)

	dl := downloader.NewDownloader()

	if err := client.Run(context.Background(), func(ctx context.Context) error {
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("auth: %w", err)
		}

		user, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("failed to get self: %w", err)
		}
		fmt.Printf("Logged in as: %s %s (@%s)\n", user.FirstName, user.LastName, user.Username)

		channels, err := FetchChannels(ctx, client.API())
		if err != nil {
			return fmt.Errorf("fetching channels: %w", err)
		}

		if len(channels) == 0 {
			fmt.Println("No channels found.")
			return nil
		}

		selected := SelectChannel(channels)
		if selected == nil {
			fmt.Println("No channel selected. Exiting.")
			return nil
		}

		if err := DownloadVideos(ctx, client.API(), dl, selected); err != nil {
			return fmt.Errorf("downloading videos: %w", err)
		}

		return nil
	}); err != nil {
		log.Fatalln(err)
	}
}

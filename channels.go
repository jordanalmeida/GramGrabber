package main

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"
)

type ChannelInfo struct {
	ID         int64
	AccessHash int64
	Title      string
	Username   string // might be empty
}

func FetchChannels(ctx context.Context, client *tg.Client) ([]ChannelInfo, error) {
	dialogs, err := client.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get dialogs: %w", err)
	}

	var channels []ChannelInfo

	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		channels = parseChats(d.Chats)
	case *tg.MessagesDialogsSlice:
		channels = parseChats(d.Chats)
	case *tg.MessagesDialogsNotModified:
		return nil, fmt.Errorf("dialogs not modified")
	}

	return channels, nil
}

func parseChats(chats []tg.ChatClass) []ChannelInfo {
	var results []ChannelInfo
	for _, chat := range chats {
		switch c := chat.(type) {
		case *tg.Channel:
			if c.Left {
				continue
			}
			results = append(results, ChannelInfo{
				ID:         c.ID,
				AccessHash: c.AccessHash,
				Title:      c.Title,
				Username:   c.Username,
			})
		case *tg.Chat:
			if c.Left {
				continue
			}
			results = append(results, ChannelInfo{
				ID:         c.ID,
				AccessHash: 0,
				Title:      c.Title,
			})
		}
	}
	return results
}

func SelectChannel(channels []ChannelInfo) *ChannelInfo {
	fmt.Println("\nAvailable Channels:")
	for i, ch := range channels {
		name := ch.Title
		if ch.Username != "" {
			name += " (@" + ch.Username + ")"
		}
		fmt.Printf("[%d] %s\n", i+1, name)
	}

	fmt.Print("\nSelect a channel (enter number): ")
	var index int
	_, err := fmt.Scanln(&index)
	if err != nil || index < 1 || index > len(channels) {
		fmt.Println("Invalid selection.")
		return nil
	}

	return &channels[index-1]
}

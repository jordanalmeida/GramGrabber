package core

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gotd/td/tg"
)

type ChannelInfo struct {
	ID         int64  `json:"id"`
	AccessHash int64  `json:"-"`
	Title      string `json:"title"`
	Username   string `json:"username"`
	IsChannel  bool   `json:"isChannel"`
}

func (c ChannelInfo) InputPeer() tg.InputPeerClass {
	if c.IsChannel {
		return &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}
	}
	return &tg.InputPeerChat{ChatID: c.ID}
}

var unsafePath = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

// FolderName returns a filesystem-safe folder name for the channel.
func (c ChannelInfo) FolderName() string {
	name := strings.TrimSpace(unsafePath.ReplaceAllString(c.Title, " "))
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = fmt.Sprintf("channel_%d", c.ID)
	}
	return name
}

func FetchChannels(ctx context.Context, api *tg.Client) ([]ChannelInfo, error) {
	dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get dialogs: %w", err)
	}

	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		return parseChats(d.Chats), nil
	case *tg.MessagesDialogsSlice:
		return parseChats(d.Chats), nil
	case *tg.MessagesDialogsNotModified:
		return nil, fmt.Errorf("dialogs not modified")
	}
	return nil, nil
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
				IsChannel:  true,
			})
		case *tg.Chat:
			if c.Left {
				continue
			}
			results = append(results, ChannelInfo{
				ID:    c.ID,
				Title: c.Title,
			})
		}
	}
	return results
}

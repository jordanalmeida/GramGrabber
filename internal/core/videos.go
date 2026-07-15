package core

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"
)

type VideoInfo struct {
	MsgID    int     `json:"msgId"`
	DocID    int64   `json:"docId"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Duration float64 `json:"duration"`
	Date     int     `json:"date"`
}

func videoFromMessage(m *tg.Message) (*VideoInfo, *tg.Document) {
	media, ok := m.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, nil
	}
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return nil, nil
	}

	var duration float64
	isVideo := false
	name := ""
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeVideo:
			isVideo = true
			duration = a.Duration
		case *tg.DocumentAttributeFilename:
			name = a.FileName
		}
	}
	if !isVideo {
		return nil, nil
	}
	if name == "" {
		name = fmt.Sprintf("%d_%d.mp4", m.ID, doc.ID)
	}
	return &VideoInfo{
		MsgID:    m.ID,
		DocID:    doc.ID,
		Name:     name,
		Size:     doc.Size,
		Duration: duration,
		Date:     m.Date,
	}, doc
}

func extractMessages(history tg.MessagesMessagesClass) ([]tg.MessageClass, error) {
	switch h := history.(type) {
	case *tg.MessagesChannelMessages:
		return h.Messages, nil
	case *tg.MessagesMessagesSlice:
		return h.Messages, nil
	case *tg.MessagesMessages:
		return h.Messages, nil
	}
	return nil, fmt.Errorf("unexpected history type: %T", history)
}

// ListVideos walks the channel history (newest first) collecting video
// documents, up to max entries.
func ListVideos(ctx context.Context, api *tg.Client, ch ChannelInfo, max int) ([]VideoInfo, error) {
	if max <= 0 {
		max = 200
	}
	peer := ch.InputPeer()

	var videos []VideoInfo
	offsetID := 0
	for len(videos) < max {
		history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			Limit:    100,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get history: %w", err)
		}
		msgs, err := extractMessages(history)
		if err != nil {
			return nil, err
		}
		if len(msgs) == 0 {
			break
		}
		lowest := 0
		for _, msg := range msgs {
			if m, ok := msg.(*tg.Message); ok {
				if lowest == 0 || m.ID < lowest {
					lowest = m.ID
				}
				if v, _ := videoFromMessage(m); v != nil {
					videos = append(videos, *v)
				}
			} else if s, ok := msg.(*tg.MessageService); ok {
				if lowest == 0 || s.ID < lowest {
					lowest = s.ID
				}
			}
		}
		if lowest == 0 || lowest == offsetID {
			break
		}
		offsetID = lowest
	}
	if len(videos) > max {
		videos = videos[:max]
	}
	return videos, nil
}

// FetchDocument re-fetches a single message to obtain a fresh document
// (file references expire, so this runs right before a download starts).
func FetchDocument(ctx context.Context, api *tg.Client, ch ChannelInfo, msgID int) (*tg.Document, *VideoInfo, error) {
	ids := []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}}

	var (
		res tg.MessagesMessagesClass
		err error
	)
	if ch.IsChannel {
		res, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash},
			ID:      ids,
		})
	} else {
		res, err = api.MessagesGetMessages(ctx, ids)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch message %d: %w", msgID, err)
	}
	msgs, err := extractMessages(res)
	if err != nil {
		return nil, nil, err
	}
	for _, msg := range msgs {
		m, ok := msg.(*tg.Message)
		if !ok || m.ID != msgID {
			continue
		}
		v, doc := videoFromMessage(m)
		if v == nil {
			return nil, nil, fmt.Errorf("message %d has no video", msgID)
		}
		return doc, v, nil
	}
	return nil, nil, fmt.Errorf("message %d not found", msgID)
}

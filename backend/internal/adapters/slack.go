package adapters

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/admin/message-router/internal/types"
	"github.com/slack-go/slack"
)

type SlackAdapter struct {
	client *slack.Client
	token  string
}

func NewSlackAdapter(token string) *SlackAdapter {
	return &SlackAdapter{
		client: slack.New(token),
		token:  token,
	}
}

func (s *SlackAdapter) GetID() string {
	return "slack"
}

func (s *SlackAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	var messages []types.Message

	// List all conversations (DMs, Private Channels, etc.)
	params := &slack.GetConversationsParameters{
		Types: []string{"public_channel", "private_channel", "im", "mpim"},
	}

	channels, _, err := s.client.GetConversationsContext(ctx, params)
	if err != nil {
		return nil, err
	}

	for _, channel := range channels {
		// Fetch history for each channel since 'since'
		historyParams := &slack.GetConversationHistoryParameters{
			ChannelID: channel.ID,
			Oldest:    timeToSlackTimestamp(since),
		}

		history, err := s.client.GetConversationHistoryContext(ctx, historyParams)
		if err != nil {
			// Log error but continue with other channels
			continue
		}

		for _, msg := range history.Messages {
			if msg.SubType != "" {
				continue // Skip join/leave/bot messages
			}

			timestamp, _ := slackTimestampToTime(msg.Timestamp)
			
			messages = append(messages, types.Message{
				ID:        msg.Timestamp,
				Source:    "slack",
				Sender:    msg.User,
				Content:   msg.Text,
				Timestamp: timestamp,
				IsPrivate: channel.IsIM || channel.IsPrivate,
				ChatName:  channel.Name,
			})
		}
	}

	return messages, nil
}

func timeToSlackTimestamp(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

func slackTimestampToTime(ts string) (time.Time, error) {
	parts := strings.Split(ts, ".")
	if len(parts) == 0 {
		return time.Now(), nil
	}
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Now(), err
	}
	return time.Unix(sec, 0), nil
}

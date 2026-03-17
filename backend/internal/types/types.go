package types

import (
	"context"
	"time"
)

type Message struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"` // Slack, Telegram, Lark
	Sender    string    `json:"sender"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsPrivate bool      `json:"is_private"`
	ChatName  string    `json:"chat_name"`
}

type Source interface {
	FetchMessages(ctx context.Context, since time.Time) ([]Message, error)
	GetID() string
}

type Destination interface {
	SendSummary(ctx context.Context, summary string) error
	GetID() string
}

type Summarizer interface {
	Summarize(ctx context.Context, messages []Message) (string, error)
}

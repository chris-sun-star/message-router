package adapters

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/admin/message-router/internal/types"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type LarkAdapter struct {
	client *lark.Client
	token  string
}

func NewLarkAdapter(token string) *LarkAdapter {
	// For user-level access, we only need the user_access_token
	return &LarkAdapter{
		client: lark.NewClient("", ""), // App ID/Secret not needed for user token calls
		token:  token,
	}
}

func (l *LarkAdapter) GetID() string {
	return "lark"
}

func (l *LarkAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	var messages []types.Message

	// 1. Get all chats the user is in
	req := larkim.NewListChatReqBuilder().
		Build()

	resp, err := l.client.Im.V1.Chat.List(ctx, req, larkcore.WithUserAccessToken(l.token))
	if err != nil {
		return nil, err
	}

	if !resp.Success() {
		return nil, fmt.Errorf("lark API error: %d %s", resp.Code, resp.Msg)
	}

	for _, item := range resp.Data.Items {
		// 2. Fetch messages for each chat
		msgReq := larkim.NewListMessageReqBuilder().
			ContainerIdType("chat").
			ContainerId(*item.ChatId).
			StartTime(strconv.FormatInt(since.UnixMilli(), 10)).
			Build()

		msgResp, err := l.client.Im.V1.Message.List(ctx, msgReq, larkcore.WithUserAccessToken(l.token))
		if err != nil {
			continue
		}

		if !msgResp.Success() {
			continue
		}

		for _, msg := range msgResp.Data.Items {
			if *msg.MsgType != "text" {
				continue // Skip non-text messages for now
			}

			// Parse content (Lark text is JSON-encoded)
			content := *msg.Body.Content
			
			ts, _ := strconv.ParseInt(*msg.CreateTime, 10, 64)
			messages = append(messages, types.Message{
				ID:        *msg.MessageId,
				Source:    "lark",
				Sender:    *msg.Sender.Id,
				Content:   content,
				Timestamp: time.UnixMilli(ts),
				IsPrivate: false, // im/v1/chat/list only returns groups
				ChatName:  *item.Name,
			})
		}
	}

	return messages, nil
}

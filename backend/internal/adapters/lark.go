package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/admin/message-router/internal/types"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type LarkAdapter struct {
	client *lark.Client
	token  string
}

func NewLarkAdapter(appID, appSecret, token string) *LarkAdapter {
	return &LarkAdapter{
		client: lark.NewClient(appID, appSecret),
		token:  token,
	}
}

func (l *LarkAdapter) GetID() string {
	return "lark"
}

func (l *LarkAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	var messages []types.Message

	// 0. Get current user info for self-filtering
	selfInfo, err := l.client.Authen.V1.UserInfo.Get(ctx, larkcore.WithUserAccessToken(l.token))
	var selfID string
	if err == nil && selfInfo.Success() {
		selfID = *selfInfo.Data.UserId
	}

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
			StartTime(strconv.FormatInt(since.UnixMilli()+1, 10)). // +1 to avoid fetching the last message again
			Build()

		msgResp, err := l.client.Im.V1.Message.List(ctx, msgReq, larkcore.WithUserAccessToken(l.token))
		if err != nil {
			continue
		}

		if !msgResp.Success() {
			continue
		}

		for _, msg := range msgResp.Data.Items {
			// Skip self messages
			if msg.Sender.Id != nil && *msg.Sender.Id == selfID {
				continue
			}

			// Lark content is JSON string: {"text":"content"}
			var contentObj struct {
				Text string `json:"text"`
			}
			json.Unmarshal([]byte(*msg.Body.Content), &contentObj)
			
			content := contentObj.Text
			
			// Media detection
			if *msg.MsgType != "text" {
				mediaPlaceholder := fmt.Sprintf("[%s]", *msg.MsgType)
				if content == "" {
					content = mediaPlaceholder
				} else {
					content = mediaPlaceholder + " " + content
				}
			}

			ts, _ := strconv.ParseInt(*msg.CreateTime, 10, 64)
			
			messages = append(messages, types.Message{
				ID:        *msg.MessageId,
				Source:    "lark",
				Sender:    *msg.Sender.Id,
				Content:   content,
				Timestamp: time.UnixMilli(ts),
				IsPrivate: false, // Defaulting to false, will investigate correct field later if needed
				ChatName:  *item.Name,
			})
		}
	}

	// 2. Map of IDs to Names for final resolution
	senderIDs := make(map[string]bool)
	for _, m := range messages {
		senderIDs[m.Sender] = true
	}

	if len(senderIDs) > 0 {
		var ids []string
		for id := range senderIDs {
			ids = append(ids, id)
		}

		// Batch get user info
		userReq := larkcontact.NewBatchUserReqBuilder().
			UserIds(ids).
			UserIdType("open_id").
			Build()
		
		userResp, err := l.client.Contact.V3.User.Batch(ctx, userReq, larkcore.WithUserAccessToken(l.token))
		if err == nil && userResp.Success() {
			nameMap := make(map[string]string)
			for _, user := range userResp.Data.Items {
				nameMap[*user.OpenId] = *user.Name
			}

			// Update names in messages
			for i := range messages {
				if name, ok := nameMap[messages[i].Sender]; ok {
					messages[i].Sender = name
				}
			}
		}
	}

	return messages, nil
}

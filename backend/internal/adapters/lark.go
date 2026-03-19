package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/admin/message-router/internal/types"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type LarkTokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type LarkAdapter struct {
	appID          string
	appSecret      string
	client         *lark.Client
	tokenData      LarkTokenData
	tokenUpdated   func(newTokenJSON string)
}

func NewLarkAdapter(appID, appSecret, tokenJSON string, onTokenUpdate func(string)) *LarkAdapter {
	var data LarkTokenData
	// Try to parse as JSON, fallback to raw string for backward compatibility
	if err := json.Unmarshal([]byte(tokenJSON), &data); err != nil {
		data = LarkTokenData{
			AccessToken: tokenJSON,
			ExpiresAt:   time.Now().Add(1 * time.Hour), // Assume 1h if unknown
		}
	}

	return &LarkAdapter{
		appID:        appID,
		appSecret:    appSecret,
		client:       lark.NewClient(appID, appSecret, lark.WithOpenBaseUrl("https://open.larksuite.com")),
		tokenData:    data,
		tokenUpdated: onTokenUpdate,
	}
}

func (l *LarkAdapter) GetID() string {
	return "lark"
}

func (l *LarkAdapter) refreshToken(ctx context.Context) error {
	if l.tokenData.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	url := "https://open.larksuite.com/open-apis/authen/v1/refresh_access_token"
	
	// We need app_access_token to refresh user_access_token
	appTokenResp, err := l.client.GetAppAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltAppAccessTokenReq{
		AppID:     l.appID,
		AppSecret: l.appSecret,
	})
	if err != nil {
		return err
	}
	if !appTokenResp.Success() {
		return fmt.Errorf("failed to get app_access_token: %d %s", appTokenResp.Code, appTokenResp.Msg)
	}

	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": l.tokenData.RefreshToken,
	}
	
	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+appTokenResp.AppAccessToken)
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AccessToken      string `json:"access_token"`
			ExpiresIn        int    `json:"expires_in"`
			RefreshToken     string `json:"refresh_token"`
			RefreshExpiresIn int    `json:"refresh_expires_in"`
		} `json:"data"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	
	if result.Code != 0 {
		return fmt.Errorf("lark refresh error: %s", result.Msg)
	}

	// Update local state
	l.tokenData.AccessToken = result.Data.AccessToken
	l.tokenData.RefreshToken = result.Data.RefreshToken
	l.tokenData.ExpiresAt = time.Now().Add(time.Duration(result.Data.ExpiresIn) * time.Second)

	// Notify callback
	if l.tokenUpdated != nil {
		newJSON, _ := json.Marshal(l.tokenData)
		l.tokenUpdated(string(newJSON))
	}

	return nil
}

func (l *LarkAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	// Check if token needs refresh (with 10 min buffer)
	if time.Now().Add(10 * time.Minute).After(l.tokenData.ExpiresAt) {
		if err := l.refreshToken(ctx); err != nil {
			fmt.Printf("Warning: failed to refresh Lark token: %v\n", err)
			// Continue anyway, maybe the token is still valid for a bit
		}
	}

	var messages []types.Message

	// 0. Get current user info for self-filtering
	selfInfo, err := l.client.Authen.V1.UserInfo.Get(ctx, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
	var selfID string
	if err == nil && selfInfo != nil && selfInfo.Success() && selfInfo.Data != nil && selfInfo.Data.UserId != nil {
		selfID = *selfInfo.Data.UserId
	}

	// 1. Get all chats the user is in
	req := larkim.NewListChatReqBuilder().
		Build()

	resp, err := l.client.Im.V1.Chat.List(ctx, req, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
	if err != nil {
		return nil, err
	}

	if !resp.Success() || resp.Data == nil || resp.Data.Items == nil {
		return nil, fmt.Errorf("lark API error: %d %s", resp.Code, resp.Msg)
	}

	for _, item := range resp.Data.Items {
		if item.ChatId == nil {
			continue
		}
		// 2. Fetch messages for each chat
		msgReq := larkim.NewListMessageReqBuilder().
			ContainerIdType("chat").
			ContainerId(*item.ChatId).
			StartTime(strconv.FormatInt(since.UnixMilli()+1, 10)).
			Build()

		msgResp, err := l.client.Im.V1.Message.List(ctx, msgReq, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
		if err != nil || !msgResp.Success() || msgResp.Data == nil || msgResp.Data.Items == nil {
			continue
		}

		for _, msg := range msgResp.Data.Items {
			// Skip self messages
			if msg.Sender == nil || msg.Sender.Id == nil || *msg.Sender.Id == selfID {
				continue
			}

			if msg.Body == nil || msg.Body.Content == nil {
				continue
			}

			var contentObj struct {
				Text string `json:"text"`
			}
			json.Unmarshal([]byte(*msg.Body.Content), &contentObj)
			
			content := contentObj.Text
			
			if msg.MsgType != nil && *msg.MsgType != "text" {
				mediaPlaceholder := fmt.Sprintf("[%s]", *msg.MsgType)
				if content == "" {
					content = mediaPlaceholder
				} else {
					content = mediaPlaceholder + " " + content
				}
			}

			if msg.CreateTime == nil || msg.MessageId == nil {
				continue
			}
			ts, _ := strconv.ParseInt(*msg.CreateTime, 10, 64)
			
			chatName := ""
			if item.Name != nil {
				chatName = *item.Name
			}

			messages = append(messages, types.Message{
				ID:        *msg.MessageId,
				Source:    "lark",
				Sender:    *msg.Sender.Id,
				Content:   content,
				Timestamp: time.UnixMilli(ts),
				IsPrivate: false, 
				ChatName:  chatName,
			})
		}
	}

	// 2. Map of IDs to Names
	senderIDs := make(map[string]bool)
	for _, m := range messages {
		senderIDs[m.Sender] = true
	}

	if len(senderIDs) > 0 {
		var ids []string
		for id := range senderIDs {
			ids = append(ids, id)
		}

		userReq := larkcontact.NewBatchUserReqBuilder().
			UserIds(ids).
			UserIdType("open_id").
			Build()
		
		userResp, err := l.client.Contact.V3.User.Batch(ctx, userReq, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
		if err == nil && userResp.Success() && userResp.Data != nil && userResp.Data.Items != nil {
			nameMap := make(map[string]string)
			for _, user := range userResp.Data.Items {
				if user.OpenId != nil && user.Name != nil {
					nameMap[*user.OpenId] = *user.Name
				}
			}

			for i := range messages {
				if name, ok := nameMap[messages[i].Sender]; ok {
					messages[i].Sender = name
				}
			}
		}
	}

	return messages, nil
}

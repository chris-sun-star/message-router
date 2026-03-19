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
	baseURL        string
	client         *lark.Client
	tokenData      LarkTokenData
	tokenUpdated   func(newTokenJSON string)
}

func NewLarkAdapter(appID, appSecret, baseURL, tokenJSON string, onTokenUpdate func(string)) *LarkAdapter {
	var data LarkTokenData
	if err := json.Unmarshal([]byte(tokenJSON), &data); err != nil {
		data = LarkTokenData{
			AccessToken: tokenJSON,
			ExpiresAt:   time.Now().Add(1 * time.Hour),
		}
	}

	return &LarkAdapter{
		appID:        appID,
		appSecret:    appSecret,
		baseURL:      baseURL,
		client:       lark.NewClient(appID, appSecret, lark.WithOpenBaseUrl(baseURL)),
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

	url := fmt.Sprintf("%s/open-apis/authen/v1/refresh_access_token", l.baseURL)
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

	l.tokenData.AccessToken = result.Data.AccessToken
	l.tokenData.RefreshToken = result.Data.RefreshToken
	l.tokenData.ExpiresAt = time.Now().Add(time.Duration(result.Data.ExpiresIn) * time.Second)

	if l.tokenUpdated != nil {
		newJSON, _ := json.Marshal(l.tokenData)
		l.tokenUpdated(string(newJSON))
	}

	return nil
}

func (l *LarkAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	if l.tokenData.RefreshToken != "" && time.Now().Add(10 * time.Minute).After(l.tokenData.ExpiresAt) {
		if err := l.refreshToken(ctx); err != nil {
			fmt.Printf("Warning: failed to refresh Lark token: %v\n", err)
		}
	}

	var messages []types.Message

	selfInfo, err := l.client.Authen.V1.UserInfo.Get(ctx, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
	var selfID string
	if err == nil && selfInfo != nil && selfInfo.Success() && selfInfo.Data != nil && selfInfo.Data.UserId != nil {
		selfID = *selfInfo.Data.UserId
	}

	// 1. Fetch Group Chats
	req := larkim.NewListChatReqBuilder().Build()
	resp, err := l.client.Im.V1.Chat.List(ctx, req, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
	if err == nil && resp.Success() && resp.Data != nil && resp.Data.Items != nil {
		for _, item := range resp.Data.Items {
			if item.ChatId == nil { continue }
			name := "Group"
			if item.Name != nil { name = *item.Name }
			msgs := l.fetchFromChat(ctx, *item.ChatId, name, false, selfID, since)
			messages = append(messages, msgs...)
		}
	}

	// 2. Fetch P2P Chats via Contact List (Workaround for Lark not listing P2P chats)
	userReq := larkcontact.NewListUserReqBuilder().DepartmentId("0").Build()
	userResp, err := l.client.Contact.V3.User.List(ctx, userReq, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
	if err == nil && userResp.Success() && userResp.Data != nil && userResp.Data.Items != nil {
		for _, user := range userResp.Data.Items {
			if user.OpenId == nil || *user.OpenId == "" { continue }
			
			// Get or Create P2P Chat ID (idempotent)
			p2pReq := larkim.NewCreateChatReqBuilder().
				UserIdType("open_id").
				Body(larkim.NewCreateChatReqBodyBuilder().
					UserIdList([]string{*user.OpenId}).
					Build()).
				Build()
			
			p2pResp, err := l.client.Im.V1.Chat.Create(ctx, p2pReq, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
			if err == nil && p2pResp.Success() && p2pResp.Data != nil && p2pResp.Data.ChatId != nil {
				userName := "Private"
				if user.Name != nil { userName = *user.Name }
				msgs := l.fetchFromChat(ctx, *p2pResp.Data.ChatId, userName, true, selfID, since)
				messages = append(messages, msgs...)
			}
		}
	}

	// Resolve Sender Names
	senderIDs := make(map[string]bool)
	for _, m := range messages { senderIDs[m.Sender] = true }
	if len(senderIDs) > 0 {
		var ids []string
		for id := range senderIDs { ids = append(ids, id) }
		nameMap := l.resolveNames(ctx, ids)
		for i := range messages {
			if name, ok := nameMap[messages[i].Sender]; ok {
				messages[i].Sender = name
			}
		}
	}

	return messages, nil
}

func (l *LarkAdapter) fetchFromChat(ctx context.Context, chatID string, chatName string, isPrivate bool, selfID string, since time.Time) []types.Message {
	var results []types.Message
	msgReq := larkim.NewListMessageReqBuilder().
		ContainerIdType("chat").
		ContainerId(chatID).
		StartTime(strconv.FormatInt(since.UnixMilli()+1, 10)).
		Build()

	msgResp, err := l.client.Im.V1.Message.List(ctx, msgReq, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
	if err != nil || !msgResp.Success() || msgResp.Data == nil || msgResp.Data.Items == nil {
		return nil
	}

	for _, msg := range msgResp.Data.Items {
		if msg.Sender == nil || msg.Sender.Id == nil || *msg.Sender.Id == selfID { continue }
		if msg.Body == nil || msg.Body.Content == nil { continue }

		var contentObj struct { Text string `json:"text"` }
		json.Unmarshal([]byte(*msg.Body.Content), &contentObj)
		content := contentObj.Text
		
		if msg.MsgType != nil && *msg.MsgType != "text" {
			mediaPlaceholder := fmt.Sprintf("[%s]", *msg.MsgType)
			if content == "" { content = mediaPlaceholder } else { content = mediaPlaceholder + " " + content }
		}

		if msg.CreateTime == nil || msg.MessageId == nil { continue }
		ts, _ := strconv.ParseInt(*msg.CreateTime, 10, 64)
		
		results = append(results, types.Message{
			ID:        *msg.MessageId,
			Source:    "lark",
			Sender:    *msg.Sender.Id,
			Content:   content,
			Timestamp: time.UnixMilli(ts),
			IsPrivate: isPrivate,
			ChatName:  chatName,
		})
	}
	return results
}

func (l *LarkAdapter) resolveNames(ctx context.Context, ids []string) map[string]string {
	nameMap := make(map[string]string)
	userReq := larkcontact.NewBatchUserReqBuilder().
		UserIds(ids).
		UserIdType("open_id").
		Build()
	
	userResp, err := l.client.Contact.V3.User.Batch(ctx, userReq, larkcore.WithUserAccessToken(l.tokenData.AccessToken))
	if err == nil && userResp.Success() && userResp.Data != nil && userResp.Data.Items != nil {
		for _, user := range userResp.Data.Items {
			if user.OpenId != nil && user.Name != nil {
				nameMap[*user.OpenId] = *user.Name
			}
		}
	}
	return nameMap
}

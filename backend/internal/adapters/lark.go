package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/admin/message-router/internal/types"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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

func (l *LarkAdapter) rawRequest(ctx context.Context, method, path string, body interface{}, query url.Values, useTenantToken bool) (json.RawMessage, error) {
	fullURL := fmt.Sprintf("%s/open-apis/%s", l.baseURL, path)
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	token := l.tokenData.AccessToken
	if useTenantToken {
		tenantTokenResp, err := l.client.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
			AppID:     l.appID,
			AppSecret: l.appSecret,
		})
		if err != nil {
			return nil, err
		}
		if !tenantTokenResp.Success() {
			return nil, fmt.Errorf("failed to get tenant token: %d %s", tenantTokenResp.Code, tenantTokenResp.Msg)
		}
		token = tenantTokenResp.TenantAccessToken
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}

	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("Lark DEBUG: Failed to decode response: %s\n", string(respBody))
		return nil, err
	}

	if result.Code != 0 {
		// Only fallback to Tenant Token for non-P2P paths or if specifically requested
		// We avoid automatic fallback here to prevent "Bot not in chat" noise if we know it's a user-only chat
		fmt.Printf("Lark DEBUG ERROR: Path: %s, Response: %s\n", path, string(respBody))
		return nil, fmt.Errorf("lark error: %d %s", result.Code, result.Msg)
	}

	return result.Data, nil
}

func (l *LarkAdapter) refreshToken(ctx context.Context) error {
	if l.tokenData.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	refreshURL := fmt.Sprintf("%s/open-apis/authen/v1/refresh_access_token", l.baseURL)
	appTokenResp, err := l.client.GetAppAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltAppAccessTokenReq{
		AppID:     l.appID,
		AppSecret: l.appSecret,
	})
	if err != nil {
		return err
	}

	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": l.tokenData.RefreshToken,
	}
	
	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", refreshURL, bytes.NewBuffer(jsonData))
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
		l.refreshToken(ctx)
	}

	var messages []types.Message

	// 0. Get self ID
	selfData, err := l.rawRequest(ctx, "GET", "authen/v1/user_info", nil, nil, false)
	var selfID string
	if err == nil {
		var s struct { UserId string `json:"user_id"` }
		json.Unmarshal(selfData, &s)
		selfID = s.UserId
		fmt.Printf("Lark: Authenticated as user ID %s\n", selfID)
	}

	// 1. Fetch ALL Chats the user is in (Groups)
	// Note: im/v1/chats with User Token returns groups the user is in.
	groupData, err := l.rawRequest(ctx, "GET", "im/v1/chats", nil, url.Values{"page_size": {"100"}}, false)
	if err == nil {
		var g struct { 
			Items []struct { 
				ChatId string `json:"chat_id"` 
				Name string `json:"name"` 
			} `json:"items"` 
		}
		json.Unmarshal(groupData, &g)
		fmt.Printf("Lark: Found %d chats in list\n", len(g.Items))
		for _, item := range g.Items {
			msgs := l.fetchFromChat(ctx, item.ChatId, item.Name, false, selfID, since)
			messages = append(messages, msgs...)
		}
	}

	// 2. We removed the contact-to-P2P creation to stop the "no title" chat spam.
	// For now, we will rely on groups. If P2P is needed, we need a better way to discover existing P2P chat IDs.

	// Resolve Names
	senderIDs := make(map[string]bool)
	for _, m := range messages { senderIDs[m.Sender] = true }
	if len(senderIDs) > 0 {
		var ids []string
		for id := range senderIDs { ids = append(ids, id) }
		
		nameReqBody := map[string]interface{}{ "user_ids": ids }
		nameData, err := l.rawRequest(ctx, "POST", "contact/v3/users/batch_get", nameReqBody, url.Values{"user_id_type": {"open_id"}}, false)
		if err == nil {
			var n struct { 
				Items []struct { 
					OpenId string `json:"open_id"` 
					Name string `json:"name"` 
				} `json:"items"` 
			}
			json.Unmarshal(nameData, &n)
			nameMap := make(map[string]string)
			for _, user := range n.Items { nameMap[user.OpenId] = user.Name }
			for i := range messages {
				if name, ok := nameMap[messages[i].Sender]; ok {
					messages[i].Sender = name
				}
			}
		}
	}

	return messages, nil
}

func (l *LarkAdapter) fetchFromChat(ctx context.Context, chatID string, chatName string, isPrivate bool, selfID string, since time.Time) []types.Message {
	nowMilli := time.Now().UnixMilli()
	startMilli := since.UnixMilli() + 1
	if startMilli >= nowMilli {
		return nil
	}

	query := url.Values{
		"container_id_type": {"chat"},
		"container_id":      {chatID},
		"start_time":        {strconv.FormatInt(startMilli, 10)},
		"end_time":          {strconv.FormatInt(nowMilli, 10)},
	}
	
	// We try with User Token first. If it fails with 40001 (unauthorized msg ability), 
	// it means the Bot must be in the group to read messages even if we use User Token.
	msgData, err := l.rawRequest(ctx, "GET", "im/v1/messages", nil, query, false)
	if err != nil {
		// Retry with Tenant Token (Bot identity) ONLY if it's a group chat
		// because the bot cannot be in a private P2P chat between two other people.
		if !isPrivate {
			msgData, err = l.rawRequest(ctx, "GET", "im/v1/messages", nil, query, true)
		}
		if err != nil {
			return nil
		}
	}

	var m struct {
		Items []struct {
			MessageId  string `json:"message_id"`
			MsgType    string `json:"msg_type"`
			CreateTime string `json:"create_time"`
			Sender     struct { Id string `json:"id"` } `json:"sender"`
			Body       struct { Content string `json:"content"` } `json:"body"`
		} `json:"items"`
	}
	json.Unmarshal(msgData, &m)

	var results []types.Message
	for _, msg := range m.Items {
		if msg.Sender.Id == selfID { continue }
		
		var contentObj struct { Text string `json:"text"` }
		json.Unmarshal([]byte(msg.Body.Content), &contentObj)
		content := contentObj.Text
		
		if msg.MsgType != "text" {
			placeholder := fmt.Sprintf("[%s]", msg.MsgType)
			if content == "" { content = placeholder } else { content = placeholder + " " + content }
		}

		ts, _ := strconv.ParseInt(msg.CreateTime, 10, 64)
		results = append(results, types.Message{
			ID:        msg.MessageId,
			Source:    "lark",
			Sender:    msg.Sender.Id,
			Content:   content,
			Timestamp: time.UnixMilli(ts),
			IsPrivate: isPrivate,
			ChatName:  chatName,
		})
	}
	return results
}

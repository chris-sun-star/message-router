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
	"github.com/admin/message-router/pkg/utils"
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

	httpClient := utils.GetHTTPClient()
	return &LarkAdapter{
		appID:        appID,
		appSecret:    appSecret,
		baseURL:      baseURL,
		client:       lark.NewClient(appID, appSecret, lark.WithOpenBaseUrl(baseURL), lark.WithHttpClient(httpClient)),
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

	client := utils.GetHTTPClient()
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
		return nil, err
	}

	if result.Code != 0 {
		fmt.Printf("Lark API Error: Path=%s Code=%d Msg=%s\n", path, result.Code, result.Msg)
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
	
	client := utils.GetHTTPClient()
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

	fmt.Printf("Lark: Fetching chats for bot (since %s)\n", since.Format(time.RFC3339))

	// 1. Fetch ALL Chats the BOT is in
	chatData, err := l.rawRequest(ctx, "GET", "im/v1/chats", nil, url.Values{"page_size": {"100"}}, true)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chats: %w", err)
	}

	var g struct { 
		Items []struct { 
			ChatId string `json:"chat_id"` 
			Name string `json:"name"` 
		} `json:"items"` 
	}
	if err := json.Unmarshal(chatData, &g); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chats: %w", err)
	}

	fmt.Printf("Lark: Found %d chats for bot\n", len(g.Items))

	for _, item := range g.Items {
		// Skip "no title" chats to avoid spamming API and focus on real groups
		if item.Name == "" {
			continue
		}
		
		fmt.Printf("Lark: Processing chat %s (ID: %s)\n", item.Name, item.ChatId)
		msgs := l.fetchFromChat(ctx, item.ChatId, item.Name, false, since)
		if len(msgs) > 0 {
			fmt.Printf("Lark: Found %d new messages in chat %s\n", len(msgs), item.Name)
		}
		messages = append(messages, msgs...)
	}

	// 2. Resolve Names
	senderIDs := make(map[string]bool)
	for _, m := range messages { senderIDs[m.Sender] = true }
	if len(senderIDs) > 0 {
		var ids []string
		for id := range senderIDs { ids = append(ids, id) }
		
		nameReqBody := map[string]interface{}{ "user_ids": ids }
		nameData, err := l.rawRequest(ctx, "POST", "contact/v3/users/batch_get", nameReqBody, url.Values{"user_id_type": {"open_id"}}, true)
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

func (l *LarkAdapter) fetchFromChat(ctx context.Context, chatID string, chatName string, isPrivate bool, since time.Time) []types.Message {
	nowSec := time.Now().Unix()
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour).Unix()
	startSec := since.Unix()
	if startSec < sevenDaysAgo {
		startSec = sevenDaysAgo
	}
	
	if startSec >= nowSec {
		return nil
	}

	fmt.Printf("Lark: Fetching messages for chat %s (since %d, end %d)\n", chatName, startSec, nowSec)

	query := url.Values{
		"container_id_type": {"chat"},
		"container_id":      {chatID},
		"start_time":        {strconv.FormatInt(startSec, 10)},
		"end_time":          {strconv.FormatInt(nowSec, 10)},
		"page_size":         {"50"},
		"sort_type":         {"ByCreateTimeAsc"},
	}
	
	msgData, err := l.rawRequest(ctx, "GET", "im/v1/messages", nil, query, true)
	if err != nil {
		return nil
	}

	var m struct {
		Items []struct {
			MessageId  string `json:"message_id"`
			MsgType    string `json:"msg_type"`
			CreateTime string `json:"create_time"`
			Sender     struct { 
				Id string `json:"id"` 
				IdType string `json:"id_type"`
			} `json:"sender"`
			Body       struct { Content string `json:"content"` } `json:"body"`
		} `json:"items"`
	}
	json.Unmarshal(msgData, &m)

	if len(m.Items) > 0 {
		fmt.Printf("Lark: API returned %d raw messages in chat %s\n", len(m.Items), chatName)
	}

	var results []types.Message
	for _, msg := range m.Items {
		// Only process messages from users, skip bot messages
		if msg.Sender.IdType != "user" {
			continue
		}
		
		var contentObj struct { Text string `json:"text"` }
		json.Unmarshal([]byte(msg.Body.Content), &contentObj)
		content := contentObj.Text
		
		fmt.Printf("Lark DEBUG: Message from %s in %s: %s (Type: %s)\n", msg.Sender.Id, chatName, content, msg.MsgType)

		if msg.MsgType != "text" {
			placeholder := fmt.Sprintf("[%s]", msg.MsgType)
			if content == "" { content = placeholder } else { content = placeholder + " " + content }
		}

		ts, _ := strconv.ParseInt(msg.CreateTime, 10, 64)
		timestamp := time.UnixMilli(ts)

		results = append(results, types.Message{
			ID:        msg.MessageId,
			Source:    "lark",
			Sender:    msg.Sender.Id,
			Content:   content,
			Timestamp: timestamp,
			IsPrivate: isPrivate,
			ChatName:  chatName,
		})
	}
	return results
}

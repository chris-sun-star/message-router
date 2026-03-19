package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func (l *LarkAdapter) rawRequest(ctx context.Context, method, path string, body interface{}, query url.Values) (json.RawMessage, error) {
	fullURL := fmt.Sprintf("%s/open-apis/%s", l.baseURL, path)
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var bodyReader *bytes.Buffer
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		bodyReader = bytes.NewBuffer(jsonBody)
	} else {
		bodyReader = bytes.NewBuffer([]byte{})
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.tokenData.AccessToken)

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

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
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
	selfData, err := l.rawRequest(ctx, "GET", "authen/v1/user_info", nil, nil)
	var selfID string
	if err == nil {
		var s struct { UserId string `json:"user_id"` }
		json.Unmarshal(selfData, &s)
		selfID = s.UserId
		fmt.Printf("Lark: Authenticated as user ID %s\n", selfID)
	} else {
		fmt.Printf("Lark: UserInfo error: %v\n", err)
	}

	// 1. Fetch Groups
	groupData, err := l.rawRequest(ctx, "GET", "im/v1/chats", nil, nil)
	if err == nil {
		var g struct { 
			Items []struct { 
				ChatId string `json:"chat_id"` 
				Name string `json:"name"` 
			} `json:"items"` 
		}
		json.Unmarshal(groupData, &g)
		fmt.Printf("Lark: Found %d group chats\n", len(g.Items))
		for _, item := range g.Items {
			msgs := l.fetchFromChat(ctx, item.ChatId, item.Name, false, selfID, since)
			messages = append(messages, msgs...)
		}
	} else {
		fmt.Printf("Lark: Chat.List error: %v\n", err)
	}

	// 2. Fetch P2P via Contacts
	contactData, err := l.rawRequest(ctx, "GET", "contact/v3/users", nil, url.Values{"department_id": {"0"}})
	if err == nil {
		var c struct { 
			Items []struct { 
				OpenId string `json:"open_id"` 
				Name string `json:"name"` 
			} `json:"items"` 
		}
		json.Unmarshal(contactData, &c)
		fmt.Printf("Lark: Found %d contacts\n", len(c.Items))
		for _, user := range c.Items {
			if user.OpenId == "" || user.OpenId == selfID { continue }
			
			p2pBody := map[string]interface{}{
				"user_id_list": []string{user.OpenId},
			}
			p2pData, err := l.rawRequest(ctx, "POST", "im/v1/chats", p2pBody, url.Values{"user_id_type": {"open_id"}})
			if err == nil {
				var p struct { ChatId string `json:"chat_id"` }
				json.Unmarshal(p2pData, &p)
				if p.ChatId != "" {
					fmt.Printf("Lark: Fetching from P2P chat with %s (%s)\n", user.Name, p.ChatId)
					msgs := l.fetchFromChat(ctx, p.ChatId, user.Name, true, selfID, since)
					messages = append(messages, msgs...)
				}
			} else {
				fmt.Printf("Lark: CreateChat error for contact %s: %v\n", user.Name, err)
			}
		}
	} else {
		fmt.Printf("Lark: User.List error: %v\n", err)
	}

	// Resolve Names
	senderIDs := make(map[string]bool)
	for _, m := range messages { senderIDs[m.Sender] = true }
	if len(senderIDs) > 0 {
		var ids []string
		for id := range senderIDs { ids = append(ids, id) }
		
		nameReqBody := map[string]interface{}{ "user_ids": ids }
		nameData, err := l.rawRequest(ctx, "POST", "contact/v3/users/batch_get", nameReqBody, url.Values{"user_id_type": {"open_id"}})
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
	fmt.Printf("Lark: Fetching messages for chat %s (ID: %s) since %s\n", chatName, chatID, since.Format(time.RFC3339))
	query := url.Values{
		"container_id_type": {"chat"},
		"container_id":      {chatID},
		"start_time":        {strconv.FormatInt(since.UnixMilli()+1, 10)},
	}
	
	msgData, err := l.rawRequest(ctx, "GET", "im/v1/messages", nil, query)
	if err != nil {
		fmt.Printf("Lark: Message.List error for %s: %v\n", chatID, err)
		return nil
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

	if len(m.Items) > 0 {
		fmt.Printf("Lark: Found %d raw messages in chat %s\n", len(m.Items), chatName)
	}

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

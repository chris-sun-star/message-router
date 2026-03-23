package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/admin/message-router/internal/types"
	"github.com/admin/message-router/pkg/utils"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type LarkTokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	OpenID       string    `json:"open_id"`
}

type LarkAdapter struct {
	appID        string
	appSecret    string
	baseURL      string
	client       *lark.Client
	tokenData    LarkTokenData
	tokenUpdated func(newTokenJSON string)
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

func (l *LarkAdapter) ensureOpenID(ctx context.Context) error {
	if l.tokenData.OpenID != "" {
		return nil
	}

	log.Printf("[Lark] OpenID is missing, attempting to fetch from user_info...")
	userInfoURL := fmt.Sprintf("%s/open-apis/authen/v1/user_info", l.baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", userInfoURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+l.tokenData.AccessToken)

	httpClient := utils.GetHTTPClient()
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID string `json:"open_id"`
			Name   string `json:"name"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf("failed to fetch user_info: %d %s", result.Code, result.Msg)
	}

	l.tokenData.OpenID = result.Data.OpenID
	log.Printf("[Lark] Successfully resolved OpenID: %s for user %s", l.tokenData.OpenID, result.Data.Name)
	
	if l.tokenUpdated != nil {
		newJSON, _ := json.Marshal(l.tokenData)
		l.tokenUpdated(string(newJSON))
	}
	return nil
}

func (l *LarkAdapter) refreshToken(ctx context.Context) error {
	if l.tokenData.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	log.Printf("[Lark] Refreshing access token...")
	
	refreshURL := fmt.Sprintf("%s/open-apis/authen/v1/refresh_access_token", l.baseURL)
	
	appTokenResp, err := l.client.GetAppAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltAppAccessTokenReq{
		AppID:     l.appID,
		AppSecret: l.appSecret,
	})
	if err != nil {
		return fmt.Errorf("failed to get app_access_token: %w", err)
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

	httpClient := utils.GetHTTPClient()
	resp, err := httpClient.Do(req)
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
			OpenID           string `json:"open_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf("lark refresh error: %d %s", result.Code, result.Msg)
	}

	l.tokenData.AccessToken = result.Data.AccessToken
	l.tokenData.RefreshToken = result.Data.RefreshToken
	l.tokenData.ExpiresAt = time.Now().Add(time.Duration(result.Data.ExpiresIn) * time.Second)
	if result.Data.OpenID != "" {
		l.tokenData.OpenID = result.Data.OpenID
	}

	if l.tokenUpdated != nil {
		newJSON, _ := json.Marshal(l.tokenData)
		l.tokenUpdated(string(newJSON))
	}

	log.Printf("[Lark] Token refreshed successfully. New expiry: %v", l.tokenData.ExpiresAt)
	return nil
}

func (l *LarkAdapter) FetchMessages(ctx context.Context, since time.Time) ([]types.Message, error) {
	if l.tokenData.RefreshToken != "" && time.Now().Add(10*time.Minute).After(l.tokenData.ExpiresAt) {
		if err := l.refreshToken(ctx); err != nil {
			log.Printf("[Lark] Error refreshing token: %v", err)
		}
	}

	// Ensure we have OpenID for mention filtering
	if err := l.ensureOpenID(ctx); err != nil {
		log.Printf("[Lark] Warning: Could not ensure OpenID: %v. Mention filtering may fail.", err)
	}

	log.Printf("[Lark] Fetching messages since %v (User OpenID: %s)", since, l.tokenData.OpenID)

	// 1. Fetch ALL Chats the BOT is in (Group chats)
	req := larkim.NewListChatReqBuilder().
		PageSize(100).
		Build()

	resp, err := l.client.Im.Chat.List(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list chats: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("lark list chats error: %d %s (request_id: %s)", resp.Code, resp.Msg, resp.RequestId())
	}

	log.Printf("[Lark] Found %d group chats for bot", len(resp.Data.Items))

	var allMessages []types.Message
	
	// We also want to check for P2P chats. P2P chats aren't easily listed by the bot
	// but we can try to fetch messages from a chat if we have the ID.
	// For now, let's process the listed groups and add more logs.

	for _, chat := range resp.Data.Items {
		chatName := "Unnamed Group"
		if chat.Name != nil && *chat.Name != "" {
			chatName = *chat.Name
		}
		
		// Fetch messages for this group. ListChat only returns groups, so isPrivate is false.
		msgs, err := l.fetchFromChat(ctx, *chat.ChatId, chatName, false, since)
		if err != nil {
			log.Printf("[Lark] Error fetching from group %s (%s): %v", chatName, *chat.ChatId, err)
			continue
		}
		
		if len(msgs) > 0 {
			allMessages = append(allMessages, msgs...)
		}
	}

	// 2. Resolve Names
	if len(allMessages) > 0 {
		log.Printf("[Lark] Total relevant messages found: %d. Resolving sender names...", len(allMessages))
		l.resolveSenderNames(ctx, allMessages)
	} else {
		log.Printf("[Lark] No relevant messages found in any of the %d chats.", len(resp.Data.Items))
	}

	return allMessages, nil
}

func (l *LarkAdapter) fetchFromChat(ctx context.Context, chatID string, chatName string, isPrivate bool, since time.Time) ([]types.Message, error) {
	nowMs := time.Now().UnixMilli()
	startMs := since.UnixMilli()
	
	sevenDaysAgoMs := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	if startMs < sevenDaysAgoMs {
		startMs = sevenDaysAgoMs
	}

	if startMs >= nowMs {
		return nil, nil
	}

	var resp *larkim.ListMessageResp
	var err error
	
	for attempt := 1; attempt <= 3; attempt++ {
		req := larkim.NewListMessageReqBuilder().
			ContainerIdType("chat").
			ContainerId(chatID).
			StartTime(strconv.FormatInt(startMs, 10)).
			EndTime(strconv.FormatInt(nowMs, 10)).
			PageSize(20). // Reduced page size for efficiency
			SortType("ByCreateTimeDesc"). // Use DESC as requested
			Build()

		resp, err = l.client.Im.Message.List(ctx, req)
		if err == nil && resp.Success() {
			break
		}
		
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled or timed out: %w", ctx.Err())
		}
		
		log.Printf("[Lark] Attempt %d failed for chat %s (%s): %v", attempt, chatName, chatID, err)
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list messages after retries: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("lark list messages error: %d %s", resp.Code, resp.Msg)
	}

	items := resp.Data.Items
	if len(items) == 0 {
		// Log only if it's a specific debug scenario, otherwise too noisy
		// log.Printf("[Lark] No messages in chat %s for the given time range.", chatName)
		return nil, nil
	}

	log.Printf("[Lark] Processing %d messages from chat %s", len(items), chatName)

	var results []types.Message
	for _, item := range items {
		// Only care about messages from users
		if item.Sender.SenderType == nil || *item.Sender.SenderType != "user" {
			continue
		}

		// Filter by mention if it's a group chat
		isMentioned := false
		if isPrivate {
			isMentioned = true
		} else if l.tokenData.OpenID != "" {
			for _, mention := range item.Mentions {
				if mention.Id != nil && *mention.Id == l.tokenData.OpenID {
					isMentioned = true
					break
				}
			}
			// Optional: add text mention fallback if needed
		}

		if !isMentioned {
			continue
		}

		// Parse content
		var content string
		if item.Body != nil && item.Body.Content != nil {
			var bodyMap map[string]interface{}
			if err := json.Unmarshal([]byte(*item.Body.Content), &bodyMap); err == nil {
				if text, ok := bodyMap["text"].(string); ok {
					content = text
				}
			}
		}

		if *item.MsgType != "text" {
			placeholder := fmt.Sprintf("[%s]", *item.MsgType)
			if content == "" {
				content = placeholder
			} else {
				content = placeholder + " " + content
			}
		}

		createTime, _ := strconv.ParseInt(*item.CreateTime, 10, 64)
		
		results = append(results, types.Message{
			ID:        *item.MessageId,
			Source:    "lark",
			Sender:    *item.Sender.Id,
			Content:   content,
			Timestamp: time.UnixMilli(createTime),
			IsPrivate: isPrivate,
			ChatName:  chatName,
		})
	}

	if len(results) > 0 {
		log.Printf("[Lark] Found %d relevant (mentioned/private) messages in %s", len(results), chatName)
	}
	return results, nil
}

func (l *LarkAdapter) resolveSenderNames(ctx context.Context, messages []types.Message) {
	senderIDs := make(map[string]bool)
	for _, m := range messages {
		senderIDs[m.Sender] = true
	}

	var ids []string
	for id := range senderIDs {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return
	}

	req := larkcontact.NewBatchUserReqBuilder().
		UserIds(ids).
		UserIdType("open_id").
		Build()

	resp, err := l.client.Contact.User.Batch(ctx, req)
	if err != nil {
		log.Printf("[Lark] Error resolving names: %v", err)
		return
	}
	if !resp.Success() {
		log.Printf("[Lark] Error resolving names API: %d %s", resp.Code, resp.Msg)
		return
	}

	nameMap := make(map[string]string)
	for _, user := range resp.Data.Items {
		if user.Name != nil {
			nameMap[*user.OpenId] = *user.Name
		}
	}

	for i := range messages {
		if name, ok := nameMap[messages[i].Sender]; ok {
			messages[i].Sender = name
		}
	}
}

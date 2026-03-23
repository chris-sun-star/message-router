package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/models"
	"github.com/admin/message-router/pkg/utils"
	"github.com/gin-gonic/gin"
)

type LarkAuthURLResponse struct {
	URL string `json:"url"`
}

func getLarkBaseURL() string {
	if config.AppConfig.Channels.Lark.Domain == "feishu" {
		return "https://open.feishu.cn"
	}
	return "https://open.larksuite.com"
}

func GetLarkAuthURL(c *gin.Context) {
	appID := config.AppConfig.Channels.Lark.AppID
	if appID == "" || appID == "your_lark_app_id" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lark App ID not configured in config.yaml"})
		return
	}

	redirectURI := c.Query("redirect_uri")
	if redirectURI == "" {
		redirectURI = "http://localhost:5173/channels"
	}

	baseURL := getLarkBaseURL()
	// Use modern scopes, remove deprecated contact:contact:readonly
	scopes := "im:message:readonly im:message.group_msg:readonly im:message.p2p_msg:readonly im:chat:readonly im:chat im:chat:read im:chat:operate_as_owner contact:user.base:readonly contact:user.id:readonly"
	
	// Use %20 instead of + for scope separation as some Feishu versions are picky
	encodedScopes := url.QueryEscape(scopes)
	encodedScopes = strings.ReplaceAll(encodedScopes, "+", "%20")

	authURL := fmt.Sprintf("%s/open-apis/authen/v1/index?app_id=%s&redirect_uri=%s&state=lark-auth&scope=%s", 
		baseURL, appID, url.QueryEscape(redirectURI), encodedScopes)

	c.JSON(http.StatusOK, LarkAuthURLResponse{URL: authURL})
}

type LarkCallbackRequest struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name" binding:"required"`
}

type LarkTokenResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken      string `json:"access_token"`
		TokenType        string `json:"token_type"`
		ExpiresIn        int    `json:"expires_in"`
		RefreshToken     string `json:"refresh_token"`
		RefreshExpiresIn int    `json:"refresh_expires_in"`
		Scope            string `json:"scope"`
		Name             string `json:"name"`
		EnName           string `json:"en_name"`
		AvatarUrl        string `json:"avatar_url"`
		OpenId           string `json:"open_id"`
		UnionId          string `json:"union_id"`
	} `json:"data"`
}

func HandleLarkCallback(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req LarkCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	appID := config.AppConfig.Channels.Lark.AppID
	appSecret := config.AppConfig.Channels.Lark.AppSecret

	appAccessToken, err := getLarkAppAccessToken(appID, appSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get app_access_token: %v", err)})
		return
	}

	baseURL := getLarkBaseURL()
	tokenURL := fmt.Sprintf("%s/open-apis/authen/v1/access_token", baseURL)
	postBody, _ := json.Marshal(map[string]string{
		"grant_type": "authorization_code",
		"code":       req.Code,
	})
	
	hReq, err := http.NewRequest("POST", tokenURL, bytes.NewBuffer(postBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hReq.Header.Set("Content-Type", "application/json")
	hReq.Header.Set("Authorization", "Bearer "+appAccessToken)
	
	client := &http.Client{}
	resp, err := client.Do(hReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	
	var larkResp LarkTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&larkResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode Lark response"})
		return
	}
	
	if larkResp.Code != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Lark error: %s", larkResp.Msg)})
		return
	}

	fmt.Printf("Lark OAuth Success! Scopes granted: %s\n", larkResp.Data.Scope)

	// Prepare token data JSON
	tokenData := struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		ExpiresAt    time.Time `json:"expires_at"`
		OpenID       string    `json:"open_id"`
	}{
		AccessToken:  larkResp.Data.AccessToken,
		RefreshToken: larkResp.Data.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(larkResp.Data.ExpiresIn) * time.Second),
		OpenID:       larkResp.Data.OpenId,
	}

	tokenDataJSON, _ := json.Marshal(tokenData)
	encryptionKey := config.AppConfig.Encryption.Key
	encryptedData, err := utils.Encrypt(string(tokenDataJSON), encryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt token"})
		return
	}

	credential := models.Credential{
		UserID:        userID,
		Name:          req.Name,
		SourceType:    models.SourceLark,
		EncryptedData: encryptedData,
	}

	if err := db.DB.Create(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save credential"})
		return
	}

	c.JSON(http.StatusCreated, credential)
}

func getLarkAppAccessToken(appID, appSecret string) (string, error) {
	baseURL := getLarkBaseURL()
	url := fmt.Sprintf("%s/open-apis/auth/v3/app_access_token/internal", baseURL)
	payload := map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	}
	
	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	var result struct {
		Code           int    `json:"code"`
		Msg            string `json:"msg"`
		AppAccessToken string `json:"app_access_token"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	if result.Code != 0 {
		return "", fmt.Errorf("lark error: %s", result.Msg)
	}
	
	return result.AppAccessToken, nil
}

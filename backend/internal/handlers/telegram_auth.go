package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/admin/message-router/internal/adapters"
	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/models"
	"github.com/admin/message-router/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
)

type TelegramAuthState struct {
	Phone          string
	PhoneCodeHash  string
	SessionStorage *session.StorageMemory
}

var (
	telegramAuthStates = make(map[uint]*TelegramAuthState)
	statesMu           sync.Mutex
)

type SendCodeRequest struct {
	Phone string `json:"phone" binding:"required"`
}

func SendTelegramCode(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := config.AppConfig.Channels.Telegram
	if cfg.APIID == 0 || cfg.APIHash == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Telegram API ID or Hash not configured in config.yaml"})
		return
	}

	storage := &session.StorageMemory{}
	client := adapters.NewTelegramClient(cfg.APIID, cfg.APIHash, storage)

	ctx := context.Background()
	var phoneCodeHash string

	err := client.Run(ctx, func(ctx context.Context) error {
		sentCode, err := client.API().AuthSendCode(ctx, &tg.AuthSendCodeRequest{
			PhoneNumber: req.Phone,
			APIID:       cfg.APIID,
			APIHash:     cfg.APIHash,
			Settings:    tg.CodeSettings{},
		})
		if err != nil {
			return err
		}

		switch res := sentCode.(type) {
		case *tg.AuthSentCode:
			phoneCodeHash = res.PhoneCodeHash
		default:
			return fmt.Errorf("unexpected sent code type: %T", sentCode)
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to send code: %v", err)})
		return
	}

	statesMu.Lock()
	telegramAuthStates[userID] = &TelegramAuthState{
		Phone:          req.Phone,
		PhoneCodeHash:  phoneCodeHash,
		SessionStorage: storage,
	}
	statesMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Code sent successfully"})
}

type VerifyCodeRequest struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name" binding:"required"`
}

func VerifyTelegramCode(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req VerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	statesMu.Lock()
	state, ok := telegramAuthStates[userID]
	statesMu.Unlock()

	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active authentication session. Call send-code first."})
		return
	}

	cfg := config.AppConfig.Channels.Telegram
	client := adapters.NewTelegramClient(cfg.APIID, cfg.APIHash, state.SessionStorage)
	ctx := context.Background()

	err := client.Run(ctx, func(ctx context.Context) error {
		_, err := client.API().AuthSignIn(ctx, &tg.AuthSignInRequest{
			PhoneNumber:   state.Phone,
			PhoneCodeHash: state.PhoneCodeHash,
			PhoneCode:     req.Code,
		})
		return err
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to verify code: %v", err)})
		return
	}

	// Load session data
	sessionData, err := state.SessionStorage.LoadSession(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load session data"})
		return
	}

	// Prepare credential data
	tData := struct {
		APIID   int    `json:"api_id"`
		APIHash string `json:"api_hash"`
		Session string `json:"session"`
	}{
		APIID:   cfg.APIID,
		APIHash: cfg.APIHash,
		Session: string(sessionData),
	}

	jsonData, _ := json.Marshal(tData)
	encryptionKey := config.AppConfig.Encryption.Key
	encryptedData, err := utils.Encrypt(string(jsonData), encryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt session"})
		return
	}

	credential := models.Credential{
		UserID:        userID,
		Name:          req.Name,
		SourceType:    models.SourceTelegram,
		EncryptedData: encryptedData,
	}

	if err := db.DB.Create(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save credential"})
		return
	}

	// Clean up state
	statesMu.Lock()
	delete(telegramAuthStates, userID)
	statesMu.Unlock()

	c.JSON(http.StatusCreated, credential)
}

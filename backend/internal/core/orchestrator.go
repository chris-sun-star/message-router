package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/admin/message-router/internal/adapters"
	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/models"
	"github.com/admin/message-router/internal/types"
	"github.com/admin/message-router/pkg/utils"
)

type Orchestrator struct {
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{}
}

func (o *Orchestrator) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.processSubscriptions(ctx)
		}
	}
}

func (o *Orchestrator) processSubscriptions(ctx context.Context) {
	var subs []models.Subscription
	if err := db.DB.Where("is_active = ?", true).Find(&subs).Error; err != nil {
		log.Println("Error fetching subscriptions:", err)
		return
	}

	for _, sub := range subs {
		if time.Since(sub.LastSyncAt).Seconds() < float64(sub.SyncInterval) {
			continue
		}
		go o.processSubscription(ctx, sub)
	}
}

func (o *Orchestrator) processSubscription(ctx context.Context, sub models.Subscription) {
	log.Printf("Processing subscription %d for user %d", sub.ID, sub.UserID)
	syncStartTime := time.Now()

	// Fetch source credential
	var srcCred models.Credential
	if err := db.DB.First(&srcCred, sub.SourceCredentialID).Error; err != nil {
		log.Printf("Error fetching source credential for sub %d: %v", sub.ID, err)
		return
	}

	encryptionKey := config.AppConfig.Encryption.Key
	// Decrypt source token
	srcToken, err := utils.Decrypt(srcCred.EncryptedData, encryptionKey)
	if err != nil {
		log.Printf("Error decrypting source token for cred %d: %v", srcCred.ID, err)
		return
	}

	// Create adapter
	var source types.Source
	switch srcCred.SourceType {
	case models.SourceSlack:
		source = adapters.NewSlackAdapter(srcToken)
	case models.SourceTelegram:
		var tData struct {
			APIID   int    `json:"api_id"`
			APIHash string `json:"api_hash"`
			Session string `json:"session"`
		}
		if err := json.Unmarshal([]byte(srcToken), &tData); err != nil {
			log.Printf("Error unmarshaling telegram creds: %v", err)
			return
		}
		source = adapters.NewTelegramAdapter(tData.APIID, tData.APIHash, tData.Session)
	case models.SourceLark:
		source = adapters.NewLarkAdapter(srcToken)
	default:
		log.Printf("Unknown source type %s", srcCred.SourceType)
		return
	}

	// Fetch messages
	messages, err := source.FetchMessages(ctx, sub.LastSyncAt)
	if err != nil {
		log.Printf("Error fetching messages for sub %d: %v", sub.ID, err)
		return
	}

	if len(messages) == 0 {
		log.Printf("No new messages for sub %d", sub.ID)
		o.updateSyncTime(sub.ID, syncStartTime)
		return
	}

	// Summarize or Format
	var finalContent string
	if sub.EnableSummarization && sub.LLMConfigID != nil {
		// Fetch LLM config
		var llmConfig models.LLMConfig
		if err := db.DB.First(&llmConfig, *sub.LLMConfigID).Error; err != nil {
			log.Printf("Error fetching LLM config for sub %d: %v", sub.ID, err)
			return
		}

		// Decrypt API key
		apiKey, err := utils.Decrypt(llmConfig.EncryptedKey, encryptionKey)
		if err != nil {
			log.Printf("Error decrypting LLM key for config %d: %v", llmConfig.ID, err)
			return
		}

		summarizer := NewLLMSummarizer(llmConfig.Provider, llmConfig.Model, apiKey)
		summary, err := summarizer.Summarize(ctx, messages)
		if err != nil {
			log.Printf("Error summarizing messages for sub %d: %v", sub.ID, err)
			return
		}
		finalContent = summary
	} else {
		// Simple formatting
		var sb strings.Builder
		sb.WriteString("### New Messages List\n\n")
		for _, m := range messages {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", m.Sender, m.Source, m.Content))
		}
		finalContent = sb.String()
	}

	// Fetch destination credential
	var destCred models.Credential
	if err := db.DB.First(&destCred, sub.DestinationCredentialID).Error; err != nil {
		log.Printf("Error fetching destination credential for sub %d: %v", sub.ID, err)
		return
	}

	destToken, err := utils.Decrypt(destCred.EncryptedData, encryptionKey)
	if err != nil {
		log.Printf("Error decrypting destination token for cred %d: %v", destCred.ID, err)
		return
	}

	// Send to destination
	dest := adapters.NewDingTalkAdapter(destToken)
	if err := dest.SendSummary(ctx, finalContent); err != nil {
		log.Printf("Error sending summary for sub %d: %v", sub.ID, err)
		return
	}

	log.Printf("Successfully processed subscription %d for user %d", sub.ID, sub.UserID)
	o.updateSyncTime(sub.ID, syncStartTime)
}

func (o *Orchestrator) updateSyncTime(subID uint, t time.Time) {
	db.DB.Model(&models.Subscription{}).Where("id = ?", subID).Update("last_sync_at", t)
}

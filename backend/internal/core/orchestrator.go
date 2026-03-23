package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/admin/message-router/internal/adapters"
	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/models"
	"github.com/admin/message-router/internal/types"
	"github.com/admin/message-router/pkg/utils"
)

const (
	BatchSize    = 10
	LockDuration = 5 * time.Minute
	WorkerCount  = 8
)

type Orchestrator struct {
	InstanceID string
	taskChan   chan models.Subscription
}

func NewOrchestrator() *Orchestrator {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	instanceID := fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
	return &Orchestrator{
		InstanceID: instanceID,
		taskChan:   make(chan models.Subscription, BatchSize*2),
	}
}

func (o *Orchestrator) Start(ctx context.Context) {
	// Start worker pool
	for i := 0; i < WorkerCount; i++ {
		go o.worker(ctx)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Initial check
	o.processSubscriptions(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.processSubscriptions(ctx)
		}
	}
}

func (o *Orchestrator) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case sub, ok := <-o.taskChan:
			if !ok {
				return
			}
			o.processSubscription(ctx, sub)
		}
	}
}

func (o *Orchestrator) processSubscriptions(ctx context.Context) {
	for {
		var candidates []models.Subscription
		now := time.Now()

		dbType := config.AppConfig.Database.Type
		query := db.DB.Where("is_active = ? AND (locked_until IS NULL OR locked_until < ?)", true, now)

		if dbType == "mysql" {
			query = query.Where("DATE_ADD(last_sync_at, INTERVAL sync_interval SECOND) <= ?", now)
		} else {
			// SQLite
			query = query.Where("datetime(last_sync_at, '+' || sync_interval || ' seconds') <= datetime(?)", now.Format("2006-01-02 15:04:05"))
		}

		if err := query.Limit(BatchSize).Find(&candidates).Error; err != nil {
			log.Println("Error fetching subscription candidates:", err)
			break
		}

		for _, sub := range candidates {
			if o.claimSubscription(sub.ID) {
				// Send to worker pool
				select {
				case o.taskChan <- sub:
				default:
					// If channel is full, we've reached our local capacity
					// The lock will eventually expire or be picked up by another instance
					log.Printf("Worker pool full, skipping sub %d for now", sub.ID)
				}
			}
		}

		numCandidates := len(candidates)
		// If the DB returned fewer than the BatchSize, we've exhausted the current "due" tasks
		if numCandidates < BatchSize {
			break
		}

		// Small delay to prevent tight loop and allow other instances to grab tasks
		time.Sleep(50 * time.Millisecond)
	}
}

func (o *Orchestrator) claimSubscription(subID uint) bool {
	now := time.Now()
	lockedUntil := now.Add(LockDuration)

	// Atomic update to claim the subscription
	result := db.DB.Model(&models.Subscription{}).
		Where("id = ? AND (locked_until IS NULL OR locked_until < ?)", subID, now).
		Updates(map[string]interface{}{
			"locked_until": lockedUntil,
			"locked_by":    o.InstanceID,
		})

	return result.RowsAffected > 0
}

func (o *Orchestrator) getLarkBaseURL() string {
	if config.AppConfig.Channels.Lark.Domain == "feishu" {
		return "https://open.feishu.cn"
	}
	return "https://open.larksuite.com"
}

func (o *Orchestrator) processSubscription(ctx context.Context, sub models.Subscription) {
	// Create a sub-context with timeout for this specific sync task
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

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
		larkCfg := config.AppConfig.Channels.Lark
		baseURL := o.getLarkBaseURL()
		source = adapters.NewLarkAdapter(larkCfg.AppID, larkCfg.AppSecret, baseURL, srcToken, func(newTokenJSON string) {
			// Re-encrypt and update database
			encrypted, err := utils.Encrypt(newTokenJSON, encryptionKey)
			if err == nil {
				db.DB.Model(&models.Credential{}).Where("id = ?", srcCred.ID).Update("encrypted_data", encrypted)
				log.Printf("Successfully refreshed and updated Lark token for cred %d", srcCred.ID)
			}
		})
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
			if m.Source == "telegram" || m.Source == "lark" {
				if m.IsPrivate {
					sb.WriteString(fmt.Sprintf("- **%s** said: %s\n", m.Sender, m.Content))
				} else {
					sb.WriteString(fmt.Sprintf("- **%s** mentioned you in group **%s**: %s\n", m.Sender, m.ChatName, m.Content))
				}
			} else {
				sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", m.Sender, m.Source, m.Content))
			}
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
	db.DB.Model(&models.Subscription{}).Where("id = ?", subID).Updates(map[string]interface{}{
		"last_sync_at": t,
		"locked_until": nil,
		"locked_by":    "",
	})
}

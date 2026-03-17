package handlers

import (
	"net/http"
	"time"

	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/models"
	"github.com/gin-gonic/gin"
)

type CreateSubscriptionRequest struct {
	SourceCredentialID      uint  `json:"source_credential_id" binding:"required"`
	DestinationCredentialID uint  `json:"destination_credential_id" binding:"required"`
	EnableSummarization     bool  `json:"enable_summarization"`
	LLMConfigID             *uint `json:"llm_config_id"`
	SyncInterval            int   `json:"sync_interval" binding:"required,min=60"`
}

func CreateSubscription(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify source credential
	var src models.Credential
	if err := db.DB.Where("id = ? AND user_id = ?", req.SourceCredentialID, userID).First(&src).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid source credential ID"})
		return
	}

	// Verify destination credential
	var dst models.Credential
	if err := db.DB.Where("id = ? AND user_id = ?", req.DestinationCredentialID, userID).First(&dst).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid destination credential ID"})
		return
	}

	// Verify LLM config if summarization is enabled
	if req.EnableSummarization && req.LLMConfigID != nil {
		var llm models.LLMConfig
		if err := db.DB.Where("id = ? AND user_id = ?", *req.LLMConfigID, userID).First(&llm).Error; err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid LLM config ID"})
			return
		}
	}

	sub := models.Subscription{
		UserID:                  userID,
		SourceCredentialID:      req.SourceCredentialID,
		DestinationCredentialID: req.DestinationCredentialID,
		EnableSummarization:     req.EnableSummarization,
		LLMConfigID:             req.LLMConfigID,
		SyncInterval:            req.SyncInterval,
		LastSyncAt:              time.Now(),
		IsActive:                true,
	}

	if err := db.DB.Create(&sub).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subscription: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, sub)
}

func ListSubscriptions(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var subs []models.Subscription
	if err := db.DB.Where("user_id = ?", userID).Find(&subs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscriptions"})
		return
	}

	c.JSON(http.StatusOK, subs)
}

func DeleteSubscription(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	id := c.Param("id")

	if err := db.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Subscription{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subscription deleted"})
}

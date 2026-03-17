package handlers

import (
	"net/http"

	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/models"
	"github.com/admin/message-router/pkg/utils"
	"github.com/gin-gonic/gin"
)

type AddLLMConfigRequest struct {
	Name     string `json:"name" binding:"required"`
	Provider string `json:"provider" binding:"required"`
	Model    string `json:"model" binding:"required"`
	APIKey   string `json:"api_key" binding:"required"`
}

func AddLLMConfig(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req AddLLMConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	encryptionKey := config.AppConfig.Encryption.Key
	encryptedKey, err := utils.Encrypt(req.APIKey, encryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt API key"})
		return
	}

	config := models.LLMConfig{
		UserID:       userID,
		Name:         req.Name,
		Provider:     req.Provider,
		Model:        req.Model,
		EncryptedKey: encryptedKey,
	}

	if err := db.DB.Create(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save LLM config"})
		return
	}

	c.JSON(http.StatusCreated, config)
}

func ListLLMConfigs(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var configs []models.LLMConfig
	if err := db.DB.Where("user_id = ?", userID).Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch LLM configs"})
		return
	}

	c.JSON(http.StatusOK, configs)
}

func DeleteLLMConfig(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	id := c.Param("id")

	if err := db.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&models.LLMConfig{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete LLM config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "LLM config deleted"})
}

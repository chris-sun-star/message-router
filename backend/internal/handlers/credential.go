package handlers

import (
	"net/http"

	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/models"
	"github.com/admin/message-router/pkg/utils"
	"github.com/gin-gonic/gin"
)

type AddCredentialRequest struct {
	Name       string            `json:"name" binding:"required"`
	SourceType models.SourceType `json:"source_type" binding:"required"`
	Token      string            `json:"token" binding:"required"`
}

func AddCredential(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req AddCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	encryptionKey := config.AppConfig.Encryption.Key
	encryptedToken, err := utils.Encrypt(req.Token, encryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt token"})
		return
	}

	credential := models.Credential{
		UserID:        userID,
		Name:          req.Name,
		SourceType:    req.SourceType,
		EncryptedData: encryptedToken,
	}

	if err := db.DB.Create(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save credential"})
		return
	}

	c.JSON(http.StatusCreated, credential)
}

func ListCredentials(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var credentials []models.Credential
	if err := db.DB.Where("user_id = ?", userID).Find(&credentials).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch credentials"})
		return
	}

	c.JSON(http.StatusOK, credentials)
}

func DeleteCredential(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	id := c.Param("id")

	if err := db.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Credential{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete credential"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Credential deleted"})
}

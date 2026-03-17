package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Email     string         `gorm:"size:255;uniqueIndex;not null" json:"email"`
	Name      string         `gorm:"size:100;not null" json:"name"`
	Password  string         `gorm:"size:255;not null" json:"-"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type LLMConfig struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	Name          string         `gorm:"size:100;not null" json:"name"`
	Provider      string         `gorm:"size:50;not null" json:"provider"` // gemini, openai, etc.
	Model         string         `gorm:"size:100;not null" json:"model"`
	EncryptedKey  string         `gorm:"type:text;not null" json:"-"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type SourceType string

const (
	SourceSlack    SourceType = "slack"
	SourceTelegram SourceType = "telegram"
	SourceLark     SourceType = "lark"
	SourceDingTalk SourceType = "dingtalk"
)

type Credential struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	Name          string         `gorm:"size:100;not null" json:"name"`
	SourceType    SourceType     `gorm:"size:50;not null" json:"source_type"`
	EncryptedData string         `gorm:"type:text;not null" json:"-"` // Encrypted JSON
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type Subscription struct {
	ID                      uint           `gorm:"primaryKey" json:"id"`
	UserID                  uint           `gorm:"not null;index" json:"user_id"`
	SourceCredentialID      uint           `gorm:"not null" json:"source_credential_id"`
	DestinationCredentialID uint           `gorm:"not null" json:"destination_credential_id"`
	EnableSummarization     bool           `gorm:"default:true" json:"enable_summarization"`
	LLMConfigID             *uint          `json:"llm_config_id"`
	LastSyncAt              time.Time      `json:"last_sync_at"`
	SyncInterval            int            `gorm:"default:300" json:"sync_interval"`
	IsActive                bool           `gorm:"default:true" json:"is_active"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	DeletedAt               gorm.DeletedAt `gorm:"index" json:"-"`
}

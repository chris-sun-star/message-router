package db

import (
	"log"

	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	dsn := config.AppConfig.Database.DSN
	if dsn == "" {
		log.Fatal("database.dsn is not set in config")
	}

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto-migrate models
	err = DB.AutoMigrate(&models.User{}, &models.Credential{}, &models.Subscription{}, &models.LLMConfig{})
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	log.Println("Database connection initialized and migrated")
}

package db

import (
	"log"
	"os"

	"github.com/admin/message-router/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN is not set")
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

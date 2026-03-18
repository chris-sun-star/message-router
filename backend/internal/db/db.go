package db

import (
	"log"

	"github.com/admin/message-router/internal/config"
	"github.com/admin/message-router/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	dbType := config.AppConfig.Database.Type
	dsn := config.AppConfig.Database.DSN

	if dbType == "" {
		dbType = "mysql" // Default to mysql for backward compatibility
	}

	if dsn == "" {
		log.Fatal("database.dsn is not set in config")
	}

	var err error
	switch dbType {
	case "mysql":
		DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "sqlite":
		DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	default:
		log.Fatalf("Unsupported database type: %s", dbType)
	}

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

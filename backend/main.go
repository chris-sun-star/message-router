package main

import (
	"context"
	"embed"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/admin/message-router/internal/core"
	"github.com/admin/message-router/internal/db"
	"github.com/admin/message-router/internal/handlers"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

//go:embed all:dist
var frontendFS embed.FS

func main() {
	// Load .env if present
	if err := godotenv.Load(".env"); err != nil {
		log.Println("Warning: No .env file found, using environment variables")
	}

	// Initialize database
	db.InitDB()

	ctx := context.Background()

	// Start Orchestrator
	orchestrator := core.NewOrchestrator()
	go orchestrator.Start(ctx)

	// Initialize router
	r := gin.Default()

	// CORS Middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// API Group
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})

		// Auth
		api.POST("/auth/register", handlers.Register)
		api.POST("/auth/login", handlers.Login)

		// Protected routes
		protected := api.Group("/")
		protected.Use(handlers.AuthMiddleware())
		{
			// Credentials
			protected.POST("/credentials", handlers.AddCredential)
			protected.GET("/credentials", handlers.ListCredentials)
			protected.DELETE("/credentials/:id", handlers.DeleteCredential)

			// Subscriptions
			protected.POST("/subscriptions", handlers.CreateSubscription)
			protected.GET("/subscriptions", handlers.ListSubscriptions)
			protected.DELETE("/subscriptions/:id", handlers.DeleteSubscription)

			// LLM Configs
			protected.POST("/llm-configs", handlers.AddLLMConfig)
			protected.GET("/llm-configs", handlers.ListLLMConfigs)
			protected.DELETE("/llm-configs/:id", handlers.DeleteLLMConfig)
		}
	}

	// Serve Frontend
	distDir, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		log.Fatal(err)
	}

	staticHandler := http.FileServer(http.FS(distDir))
	
	// Handle SPA routing: if file doesn't exist, serve index.html
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api") {
			c.JSON(404, gin.H{"error": "Not Found"})
			return
		}

		// Check if file exists in embed FS
		f, err := distDir.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			f.Close()
			staticHandler.ServeHTTP(c.Writer, c.Request)
			return
		}

		// Otherwise serve index.html for SPA routing
		indexFile, _ := distDir.Open("index.html")
		defer indexFile.Close()
		http.ServeContent(c.Writer, c.Request, "index.html", time.Now(), indexFile.(io.ReadSeeker))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

package main

import (
	"log"
	"arcana-ai/internal/config"
	"arcana-ai/internal/handlers"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Initialize Gin router
	router := gin.Default()

	// 3. Register routes
	handlers.RegisterHealthRoutes(router)
	// TODO: handlers.RegisterTelegramRoutes(router, cfg)
	// TODO: handlers.RegisterTGWebAppRoutes(router, cfg)

	// 4. Start server
	log.Printf("Starting server on :%s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

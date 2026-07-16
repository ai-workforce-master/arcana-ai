package main

import (
	"log"

	"arcana-ai/internal/config"
	"arcana-ai/internal/handlers"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	router := gin.Default()

	h, err := handlers.NewHandler(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize handlers: %v", err)
	}
	h.RegisterRoutes(router)

	log.Printf("Starting server on :%s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

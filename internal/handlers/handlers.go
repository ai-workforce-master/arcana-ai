package handlers

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// RegisterHealthRoutes регистрирует маршруты для проверки здоровья сервиса
func RegisterHealthRoutes(router *gin.Engine) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "arcana-ai",
			"version": "0.1.0",
		})
	})
}

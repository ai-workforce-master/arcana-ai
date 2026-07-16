package handlers

import (
	"net/http"

	"arcana-ai/internal/config"
	"arcana-ai/internal/rag"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	cfg *config.Config
	rag *rag.Client
}

func NewHandler(cfg *config.Config) (*Handler, error) {
	ragClient, err := rag.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Handler{cfg: cfg, rag: ragClient}, nil
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/health", h.HealthCheck)
	router.GET("/api/test-rag", h.TestRAG)
}

func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "arcana-ai",
		"version": "0.1.0",
	})
}

func (h *Handler) TestRAG(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	results, err := h.rag.Search(c.Request.Context(), query, 3)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"results": results,
	})
}

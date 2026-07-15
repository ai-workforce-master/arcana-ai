package config

import (
	"github.com/joho/godotenv"
	"os"
)

type Config struct {
	ServerPort        string
	AITunnelAPIKey    string
	AITunnelBaseURL   string
	TelegramBotToken  string
	YooKassaShopID    string
	YooKassaSecretKey string
	ModerationSalt    string
	RedisURL          string
	PostgresDSN       string
	QdrantURL         string
	EmbeddingsSvcURL  string
}

func Load() (*Config, error) {
	_ = godotenv.Load() // Ignore error if .env is missing (e.g., in production with real env vars)

	return &Config{
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		AITunnelAPIKey:    getEnv("AITUNNEL_API_KEY", ""),
		AITunnelBaseURL:   getEnv("AITUNNEL_BASE_URL", "https://api.aitunnel.ru/v1/"),
		TelegramBotToken:  getEnv("TELEGRAM_BOT_TOKEN", ""),
		YooKassaShopID:    getEnv("YOOKASSA_SHOP_ID", ""),
		YooKassaSecretKey: getEnv("YOOKASSA_SECRET_KEY", ""),
		ModerationSalt:    getEnv("MODERATION_SALT", "default_salt"),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379/0"),
		PostgresDSN:       getEnv("POSTGRES_DSN", "postgres://arcana:arcana_password@localhost:5432/arcana_db?sslmode=disable"),
		QdrantURL:         getEnv("QDRANT_URL", "http://localhost:6333"),
		EmbeddingsSvcURL:  getEnv("EMBEDDINGS_SVC_URL", "http://localhost:8000"),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

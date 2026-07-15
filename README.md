# Arcana AI Tarot

AI-сервис гадания на Таро с голосовым интерфейсом и Telegram Mini App.

## 🛠 Стек
- **Backend**: Go (Gin), модульный монолит
- **Frontend**: React 18, TypeScript, Vite, Framer Motion
- **AI**: AITunnel (gpt-4o-mini, gpt-4o-mini-transcribe, gpt-4o-mini-tts)
- **Embeddings**: Python sidecar (`paraphrase-multilingual-MiniLM-L12-v2`) с фоллбэком на AITunnel
- **Infrastructure**: Docker Compose (Postgres, Redis, Qdrant)

## 🚀 Локальный запуск (Development)

1. **Клонирование и подготовка**:

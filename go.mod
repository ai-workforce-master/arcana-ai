module arcana-ai

go 1.24

require (
	github.com/gin-gonic/gin v1.10.0
	github.com/joho/godotenv v1.5.1
	github.com/qdrant/go-client v1.12.0
	github.com/sashabaranov/go-openai v1.41.2
)

// Железно фиксируем версию, запрещая go mod tidy обновлять её
replace github.com/qdrant/go-client => github.com/qdrant/go-client v1.12.0

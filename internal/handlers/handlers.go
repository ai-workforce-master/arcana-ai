package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"arcana-ai/internal/config"
	"arcana-ai/internal/payments"
	"arcana-ai/internal/rag"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/sashabaranov/go-openai"
)

type Handler struct {
	cfg        *config.Config
	db         *sql.DB
	rag        *rag.Client
	openAICli  *openai.Client
	promptFile string
	yooKassa   *payments.YooKassaService
}

func NewHandler(cfg *config.Config) (*Handler, error) {
	// 1. Подключение к базе данных
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	log.Println("Database connected successfully")

	// 2. Инициализация RAG
	ragClient, err := rag.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	// 3. Инициализация OpenAI
	openaiConfig := openai.DefaultConfig(cfg.AITunnelAPIKey)
	openaiConfig.BaseURL = cfg.AITunnelBaseURL
	openAICli := openai.NewClientWithConfig(openaiConfig)

	// 4. Файл промпта
	promptFile := os.Getenv("PROMPT_FILE")
	if promptFile == "" {
		promptFile = "prompts/tarot_reading.txt"
	}

	// 5. Инициализация ЮKassa
	yooKassa, err := payments.NewYooKassaService(db)
	if err != nil {
		log.Printf("Warning: YooKassa not initialized: %v", err)
	} else {
		log.Println("YooKassa service initialized successfully")
	}

	return &Handler{
		cfg:        cfg,
		db:         db,
		rag:        ragClient,
		openAICli:  openAICli,
		promptFile: promptFile,
		yooKassa:   yooKassa,
	}, nil
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/health", h.HealthCheck)
	router.GET("/api/test-rag", h.TestRAG)
	router.POST("/api/read-spread", h.ReadSpread)
	router.POST("/api/create-payment", h.CreatePayment)
	router.POST("/api/payment-webhook", h.PaymentWebhook)
	router.GET("/api/payment-status/:id", h.PaymentStatus)
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

type CardRequest struct {
	Name       string `json:"name" binding:"required"`
	IsReversed bool   `json:"is_reversed"`
}

type ReadSpreadRequest struct {
	Question string        `json:"question" binding:"required"`
	Spread   string        `json:"spread"`
	Cards    []CardRequest `json:"cards" binding:"required"`
}

type EnrichedCardData struct {
	rag.CardData
	IsReversed bool
	Position   string // Добавлено для передачи позиции в промпт
}

type CardInterpretation struct {
	Card           CardRequest `json:"card"`
	Interpretation string      `json:"interpretation"`
}

type ReadSpreadResponse struct {
	Intro      string               `json:"intro"`
	Cards      []CardInterpretation `json:"cards"`
	Conclusion string               `json:"conclusion"`
}

// LLMCardInterpretation описывает структуру ответа LLM для одной карты
type LLMCardInterpretation struct {
	CardName       string `json:"card_name"`
	Interpretation string `json:"interpretation"`
}

// LLMResponse описывает строгую JSON-схему ответа от LLM
type LLMResponse struct {
	Intro      string                  `json:"intro"`
	Cards      []LLMCardInterpretation `json:"cards"`
	Conclusion string                  `json:"conclusion"`
}

func (h *Handler) loadSystemPrompt() string {
	data, err := os.ReadFile(h.promptFile)
	if err != nil {
		return "Ты — опытный таролог. Давай глубокие и эмпатичные интерпретации раскладов."
	}
	return strings.TrimSpace(string(data))
}

func (h *Handler) ReadSpread(c *gin.Context) {
	var req ReadSpreadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cardNames := make([]string, len(req.Cards))
	for i, card := range req.Cards {
		cardNames[i] = card.Name
	}

	positions := getSpreadPositions(req.Spread, len(req.Cards))

	cardDataList, err := h.rag.GetCardsData(c.Request.Context(), cardNames, positions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var enrichedCards []EnrichedCardData
	for i, card := range cardDataList {
		enrichedCards = append(enrichedCards, EnrichedCardData{
			CardData:   card,
			IsReversed: req.Cards[i].IsReversed,
			Position:   positions[i],
		})
	}

	systemPrompt := h.loadSystemPrompt()
	userPrompt := buildLLMPrompt(req.Question, req.Spread, enrichedCards)

	maxTokens := 8192

	resp, err := h.openAICli.CreateChatCompletion(
		c.Request.Context(),
		openai.ChatCompletionRequest{
			Model: "gpt-4o",
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
				{Role: openai.ChatMessageRoleUser, Content: userPrompt},
			},
			Temperature: 0.7,
			MaxTokens:   maxTokens,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("LLM generation failed: %v", err)})
		return
	}

	llmResponse := resp.Choices[0].Message.Content

	// 1. Очистка ответа от возможных markdown-обёрток (```json ... ```)
	cleanJSON := strings.TrimSpace(llmResponse)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	cleanJSON = strings.TrimSpace(cleanJSON)

	// 2. Парсинг строгого JSON
	var parsedResp LLMResponse
	if err := json.Unmarshal([]byte(cleanJSON), &parsedResp); err != nil {
		log.Printf("JSON parse error: %v, raw response: %s", err, llmResponse)
		// Fallback: если LLM всё же сломал формат, возвращаем сырой текст, чтобы не ломать фронтенд
		c.JSON(http.StatusOK, ReadSpreadResponse{
			Intro:      "Извините, произошла ошибка форматирования ответа LLM.",
			Cards:      []CardInterpretation{},
			Conclusion: llmResponse,
		})
		return
	}

	// 3. Надёжное сопоставление карт по имени (а не по порядку абзацев!)
	var responseCards []CardInterpretation
	for _, reqCard := range req.Cards {
		interpretation := "Энергия этой карты тесно переплетена с общим смыслом расклада и дополняет его."
		
		// Ищем толкование именно для этой карты (нечувствительно к регистру и пробелам)
		for _, llmCard := range parsedResp.Cards {
			if strings.EqualFold(strings.TrimSpace(llmCard.CardName), strings.TrimSpace(reqCard.Name)) {
				interpretation = llmCard.Interpretation
				break
			}
		}
		
		responseCards = append(responseCards, CardInterpretation{
			Card:           reqCard,
			Interpretation: interpretation,
		})
	}

	// 4. Отдаём идеальный, структурированный ответ
	c.JSON(http.StatusOK, ReadSpreadResponse{
		Intro:      parsedResp.Intro,
		Cards:      responseCards,
		Conclusion: parsedResp.Conclusion,
	})
}

func getSpreadPositions(spreadName string, cardCount int) []string {
	spread := strings.ToLower(spreadName)
	if strings.Contains(spread, "одна карт") || strings.Contains(spread, "карта дня") {
		return []string{"Послание дня"}
	}
	if strings.Contains(spread, "да или нет") {
		return []string{"Факторы ДА", "Факторы НЕТ", "Итог"}
	}
	if strings.Contains(spread, "три карт") || strings.Contains(spread, "триплет") {
		return []string{"Прошлое", "Настоящее", "Будущее"}
	}
	if strings.Contains(spread, "крест") && !strings.Contains(spread, "кельтский") {
		return []string{"Центр (суть)", "Слева (препятствия)", "Справа (поддержка)", "Сверху (итог)"}
	}
	if strings.Contains(spread, "слепое пятно") {
		return []string{"Осознаваемое", "Мистер Икс (скрытое)", "Тень", "Слепое пятно"}
	}
	if strings.Contains(spread, "пятикарточн") {
		return []string{"Прошлое", "Настоящее", "Скрытые влияния", "Совет", "Результат"}
	}
	if strings.Contains(spread, "ангел") || strings.Contains(spread, "голос") {
		return []string{"Истоки", "Текущее состояние", "Совет", "Наставление ангела"}
	}
	if strings.Contains(spread, "пирамида") {
		return []string{"Прошлые влияния", "Текущая ситуация", "Внешние факторы", "Внутренние процессы", "Вызовы", "Вершина (итог)"}
	}
	if strings.Contains(spread, "алхимик") {
		return []string{"Прошлое", "Настоящее", "Будущее", "Ближайшее будущее", "Промежуточное", "Отдаленное"}
	}
	if strings.Contains(spread, "подкова") {
		return []string{"Прошлое", "Настоящее", "Скрытые влияния", "Препятствия", "Окружение", "Совет", "Результат"}
	}
	if strings.Contains(spread, "звезда") || strings.Contains(spread, "чакр") {
		return []string{"Корневая чакра", "Сакральная", "Солнечное сплетение", "Сердечная", "Горловая", "Третий глаз", "Коронная"}
	}
	if strings.Contains(spread, "партнерство") {
		return []string{"Сигнификатор", "Сознание спрашивающего", "Эмоции спрашивающего", "Внешняя сторона (спрашивающий)", "Сознание партнера", "Эмоции партнера", "Внешняя сторона (партнер)"}
	}
	if strings.Contains(spread, "кельтский") {
		return []string{"Ситуация", "Препятствие", "Далекое прошлое", "Недавнее прошлое", "Возможное будущее", "Ближайшее будущее", "Ваше состояние", "Окружение", "Надежды и страхи", "Результат"}
	}
	if strings.Contains(spread, "колесо") {
		return []string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}
	}
	positions := make([]string, cardCount)
	for i := 0; i < cardCount; i++ {
		positions[i] = fmt.Sprintf("Позиция %d", i+1)
	}
	return positions
}

func buildLLMPrompt(question, spreadName string, cards []EnrichedCardData) string {
	var sb strings.Builder
	sb.WriteString("Проинтерпретируй расклад Таро на основе следующих данных.\n\n")
	sb.WriteString(fmt.Sprintf("**Вопрос кверента:** %s\n", question))
	if spreadName != "" {
		sb.WriteString(fmt.Sprintf("**Используемый расклад:** %s\n\n", spreadName))
	} else {
		sb.WriteString("\n")
	}
	sb.WriteString("**Выпавшие карты и их позиции:**\n\n")

	for _, card := range cards {
		orientationText := "в прямом положении"
		meaningText := card.Upright
		
		if card.IsReversed {
			orientationText = "в перевёрнутом положении"
			meaningText = card.Reversed
			if meaningText == "" {
				meaningText = "Специфическое перевёрнутое значение не указано, трактуй как блокировку или искажение энергии прямой позиции."
			}
		}

		sb.WriteString(fmt.Sprintf("- Позиция: %s\n", card.Position))
		sb.WriteString(fmt.Sprintf("  Карта: %s (%s)\n", card.Name, orientationText))
		sb.WriteString(fmt.Sprintf("  Значение: %s\n", meaningText))
		sb.WriteString(fmt.Sprintf("  Совет карты: %s\n\n", card.Advice))
	}

	sb.WriteString("**ИНСТРУКЦИЯ ПО ФОРМАТУ ОТВЕТА (СТРОГО СОБЛЮДАЙ):**\n")
	sb.WriteString("Ты должен ответить ТОЛЬКО валидным JSON-объектом. Никакого текста до или после JSON. Никаких markdown-обёрток (например, ```json).\n")
	sb.WriteString("Используй строго следующую схему JSON:\n")
	sb.WriteString(`{
  "intro": "Краткое введение в энергию расклада и суть вопроса (2-5 предложения).",
  "cards": [
    {
      "card_name": "Точное название карты из списка выше",
      "interpretation": "Толкование этой карты в её конкретной позиции с учётом прямого/перевёрнутого положения и влияния других карт расклада (3-7 предложений)."
    }
  ],
  "conclusion": "Итоговый совет, резюме или практический следующий шаг для кверента (2-5 предложений)."
}`)
	return sb.String()
}

// ==========================================
// ЮKassa Payment Endpoints
// ==========================================

func (h *Handler) CreatePayment(c *gin.Context) {
	if h.yooKassa == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Payment service not available"})
		return
	}

	var req struct {
		SessionID  string  `json:"session_id" binding:"required"`
		SpreadName string  `json:"spread_name" binding:"required"`
		Amount     float64 `json:"amount" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	confirmationURL, paymentID, err := h.yooKassa.CreatePayment(c.Request.Context(), h.db, req.SessionID, req.SpreadName, req.Amount)
	if err != nil {
		log.Printf("Failed to create payment: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_id":       paymentID,
		"confirmation_url": confirmationURL,
	})
}

func (h *Handler) PaymentWebhook(c *gin.Context) {
	if h.yooKassa == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Payment service not available"})
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
		return
	}

	signature := c.GetHeader("X-Content-Signature")

	if err := h.yooKassa.HandleWebhook(c.Request.Context(), h.db, body, signature); err != nil {
		log.Printf("Webhook error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Webhook processing failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) PaymentStatus(c *gin.Context) {
	if h.yooKassa == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Payment service not available"})
		return
	}

	paymentID := c.Param("id")
	log.Printf("DEBUG PaymentStatus: Received request for ID = '%s' (length: %d)", paymentID, len(paymentID))

	if paymentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment ID required"})
		return
	}

	payment, err := h.yooKassa.GetPaymentStatus(c.Request.Context(), h.db, paymentID)
	if err != nil {
		log.Printf("ERROR PaymentStatus: Returning 404 due to: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	// Корректная обработка sql.NullString для receipt_url
	receiptURL := ""
	if payment.ReceiptURL.Valid {
		receiptURL = payment.ReceiptURL.String
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_id":  payment.ID,
		"status":      payment.Status,
		"amount":      payment.Amount,
		"spread_name": payment.SpreadName,
		"paid_at":     payment.PaidAt,
		"receipt_url": receiptURL,
	})
}

// StartBackgroundWorkers запускает фоновые задачи (например, воркер ФНС)
func (h *Handler) StartBackgroundWorkers(ctx context.Context) {
	if h.yooKassa != nil {
		h.yooKassa.StartFNSWorker(ctx)
	}
}

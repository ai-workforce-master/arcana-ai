package handlers

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"arcana-ai/internal/config"
	"arcana-ai/internal/rag"
	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
)

type Handler struct {
	cfg        *config.Config
	rag        *rag.Client
	openAICli  *openai.Client
	promptFile string
}

func NewHandler(cfg *config.Config) (*Handler, error) {
	ragClient, err := rag.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	openaiConfig := openai.DefaultConfig(cfg.AITunnelAPIKey)
	openaiConfig.BaseURL = cfg.AITunnelBaseURL
	
	openAICli := openai.NewClientWithConfig(openaiConfig)

	promptFile := os.Getenv("PROMPT_FILE")
	if promptFile == "" {
		promptFile = "prompts/tarot_reading.txt"
	}

	return &Handler{
		cfg:        cfg,
		rag:        ragClient,
		openAICli:  openAICli,
		promptFile: promptFile,
	}, nil
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	router.GET("/health", h.HealthCheck)
	router.GET("/api/test-rag", h.TestRAG)
	router.POST("/api/read-spread", h.ReadSpread)
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

	// УЛУЧШЕННЫЙ ПАРСИНГ С РЕГУЛЯРНЫМИ ВЫРАЖЕНИЯМИ
	paragraphs := strings.Split(llmResponse, "\n\n")
	var cleanParagraphs []string
	
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		
		// 1. Удаляем markdown заголовки (#, ##, ###)
		p = regexp.MustCompile(`(?m)^#+\s*`).ReplaceAllString(p, "")
		
		// 2. Удаляем нумерацию в начале (1., 2., 12. и т.д.)
		p = regexp.MustCompile(`(?m)^\d+\.\s*`).ReplaceAllString(p, "")
		
		// 3. Удаляем ключевые слова в начале строки (Вступление, Итог и т.д., с возможными двоеточиями)
		p = regexp.MustCompile(`(?i)^(вступление|вывод|заключение|итог|интерпретация)[\s:]*`).ReplaceAllString(p, "")
		
		// 4. Убираем жирный шрифт (**) в начале и конце строки
		p = strings.TrimPrefix(p, "**")
		p = strings.TrimSuffix(p, "**")
		
		p = strings.TrimSpace(p)
		if p != "" {
			cleanParagraphs = append(cleanParagraphs, p)
		}
	}

	intro := ""
	conclusion := ""
	if len(cleanParagraphs) > 0 {
		intro = cleanParagraphs[0]
	}
	if len(cleanParagraphs) > 1 {
		conclusion = cleanParagraphs[len(cleanParagraphs)-1]
	}

	middleParagraphs := []string{}
	if len(cleanParagraphs) > 2 {
		middleParagraphs = cleanParagraphs[1 : len(cleanParagraphs)-1]
	} else if len(cleanParagraphs) == 2 {
		middleParagraphs = cleanParagraphs[1:]
	}

	var responseCards []CardInterpretation
	for i, card := range req.Cards {
		interpretation := ""
		if i < len(middleParagraphs) {
			interpretation = middleParagraphs[i]
		} else {
			interpretation = "Энергия этой карты тесно переплетена с общим смыслом расклада и дополняет его."
		}
		
		responseCards = append(responseCards, CardInterpretation{
			Card: CardRequest{
				Name:       card.Name,
				IsReversed: card.IsReversed,
			},
			Interpretation: interpretation,
		})
	}

	c.JSON(http.StatusOK, ReadSpreadResponse{
		Intro:      intro,
		Cards:      responseCards,
		Conclusion: conclusion,
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

	sb.WriteString("Проинтерпретируй расклад на основе следующих данных.\n\n")
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

		sb.WriteString(fmt.Sprintf("- %s: «%s» (%s)\n", card.Position, card.Name, orientationText))
		sb.WriteString(fmt.Sprintf("  *Значение:* %s\n", meaningText))
		sb.WriteString(fmt.Sprintf("  *Совет карты:* %s\n", card.Advice))
	}

	sb.WriteString("\n**Формат ответа (СТРОГО СОБЛЮДАЙ):**\n")
	sb.WriteString("Раздели ответ на части ДВОЙНЫМ переносом строки (\\n\\n).\n")
	sb.WriteString("НЕ используй заголовки (##), жирный шрифт (**) или нумерацию (1., 2.) для названий частей. Пиши просто текстом.\n")
	sb.WriteString("Первый абзац: Вступление (1-2 абзаца), кратко опиши общую энергетику расклада.\n")
	for _, card := range cards {
		sb.WriteString(fmt.Sprintf("Затем абзац(ы): Интерпретация для карты '%s' в позиции '%s' (1-2 абзаца, подробно).\n", card.Name, card.Position))
	}
	sb.WriteString("Последний абзац: Итоговый вывод (ОБЯЗАТЕЛЬНО 2-3 развёрнутых абзаца): обобщи все карты, дай глубокий анализ, конкретные рекомендации и поддержку кверенту.\n")
	sb.WriteString("Не используй маркированные списки внутри этих абзацев, только сплошной текст.")

	return sb.String()
}

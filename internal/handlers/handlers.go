package handlers

import (
	"fmt"
	"net/http"
	"strings"

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

// ReadSpreadRequest описывает ожидаемый JSON от фронтенда/бота
type ReadSpreadRequest struct {
	Question string   `json:"question" binding:"required"`
	Spread   string   `json:"spread"`
	Cards    []string `json:"cards" binding:"required"`
}

// ReadSpread — новый эндпоинт: ищет карты по именам и собирает промпт для LLM
func (h *Handler) ReadSpread(c *gin.Context) {
	var req ReadSpreadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 1. Получаем названия позиций из базы знаний (на основе Расклады_Таро.docx)
	positions := getSpreadPositions(req.Spread, len(req.Cards))

	// 2. Получаем данные карт из Qdrant (точечный поиск по имени каждой карты)
	cardData, err := h.rag.GetCardsData(c.Request.Context(), req.Cards, positions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 3. Формируем промпт для LLM
	prompt := buildLLMPrompt(req.Question, req.Spread, cardData)

	// TODO: Здесь будет вызов LLM через go-openai
	// llmResponse, err := h.llm.Generate(c.Request.Context(), prompt)

	// Пока возвращаем сформированный промпт для оценки качества
	c.JSON(http.StatusOK, gin.H{
		"question":       req.Question,
		"spread":         req.Spread,
		"prompt_for_llm": prompt,
		"cards_found":    len(cardData),
	})
}

// getSpreadPositions возвращает названия позиций на основе данных из Расклады_Таро.docx
func getSpreadPositions(spreadName string, cardCount int) []string {
	spread := strings.ToLower(spreadName)

	// 1. Одна карта (Карта дня)
	if strings.Contains(spread, "одна карт") || strings.Contains(spread, "карта дня") || strings.Contains(spread, "одной карт") {
		return []string{"Послание дня, основная энергия или тема"}
	}

	// 2. Да или Нет (3 карты)
	if strings.Contains(spread, "да или нет") || strings.Contains(spread, "да/нет") {
		return []string{
			"Факторы ДА — что поддерживает положительный ответ",
			"Факторы НЕТ — что указывает на препятствия",
			"Итог — окончательный ответ и рекомендация",
		}
	}

	// 3. Три карты / Триплет
	if strings.Contains(spread, "три карт") || strings.Contains(spread, "триплет") {
		return []string{
			"Прошлое — что привело к текущей ситуации",
			"Настоящее — текущее положение дел",
			"Будущее — куда движется ситуация",
		}
	}

	// 4. Крест (4 карты)
	if strings.Contains(spread, "крест") && !strings.Contains(spread, "кельтский") {
		return []string{
			"Центр — суть ситуации, основная тема",
			"Слева — что работает против вас, препятствия",
			"Справа — что работает в вашу пользу, поддержка",
			"Сверху — возможный итог, к чему всё идёт",
		}
	}

	// 5. Пятикарточный расклад
	if strings.Contains(spread, "пятикарточн") || strings.Contains(spread, "пять карт") {
		return []string{
			"Прошлое — события и опыт, повлиявшие на ситуацию",
			"Настоящее — текущее положение дел",
			"Скрытые влияния — то, что работает за кулисами",
			"Совет — что делать для лучшего результата",
			"Возможный результат — вероятное развитие событий",
		}
	}

	// 6. Подкова (7 карт)
	if strings.Contains(spread, "подкова") {
		return []string{
			"Прошлое — что привело к текущей ситуации",
			"Настоящее — текущее положение дел",
			"Скрытые влияния — неосознанные факторы, тайны",
			"Препятствия — что мешает достижению цели",
			"Окружение — влияние других людей, внешние обстоятельства",
			"Совет — лучший способ действий",
			"Результат — вероятный итог",
		}
	}

	// 7. Звезда / Чакральный расклад (7 карт)
	if strings.Contains(spread, "звезда") || strings.Contains(spread, "чакр") {
		return []string{
			"Корневая чакра — материальная основа, безопасность",
			"Сакральная чакра — эмоции, творчество",
			"Чакра солнечного сплетения — личная сила, воля",
			"Сердечная чакра — любовь, отношения",
			"Горловая чакра — коммуникация, самовыражение",
			"Чакра третьего глаза — интуиция, мудрость",
			"Коронная чакра — духовность, связь с высшим Я",
		}
	}

	// 8. Пирамида (6 карт)
	if strings.Contains(spread, "пирамида") {
		return []string{
			"Прошлые влияния (основа)",
			"Текущая ситуация (основа)",
			"Внешние факторы (основа)",
			"Внутренние процессы (развитие)",
			"Вызовы и возможности (развитие)",
			"Конечный итог, главный урок (вершина)",
		}
	}

	// 9. Кельтский крест (10 карт)
	if strings.Contains(spread, "кельтский") {
		return []string{
			"Ситуация — суть проблемы, центральная тема",
			"Препятствие — что блокирует ситуацию",
			"Далёкое прошлое — глубинные причины, истоки",
			"Недавнее прошлое — что привело к текущему состоянию",
			"Возможное будущее — потенциальное развитие",
			"Ближайшее будущее — что произойдёт вскоре",
			"Ваше состояние — ваша позиция, внутреннее состояние",
			"Окружение — влияние других людей, внешние факторы",
			"Надежды и страхи — ваши ожидания и опасения",
			"Результат — конечный итог ситуации",
		}
	}

	// 10. Колесо года (12 карт)
	if strings.Contains(spread, "колесо года") || strings.Contains(spread, "колесо") {
		return []string{
			"Январь — начало года, новые начинания",
			"Февраль — внутреннее развитие",
			"Март — пробуждение, активность",
			"Апрель — рост, развитие",
			"Май — расцвет, реализация",
			"Июнь — пик активности",
			"Июль — наслаждение результатами",
			"Август — сбор урожая",
			"Сентябрь — переоценка, баланс",
			"Октябрь — трансформация",
			"Ноябрь — отпускание, очищение",
			"Декабрь — завершение цикла",
		}
	}

	// 11. Алхимик (6 карт)
	if strings.Contains(spread, "алхимик") {
		return []string{
			"Прошлое",
			"Настоящее",
			"Будущее",
			"Ближайшее будущее",
			"Промежуточное будущее",
			"Отдалённое будущее",
		}
	}

	// 12. Слепое пятно (4 карты)
	if strings.Contains(spread, "слепое пятно") {
		return []string{
			"Осознаваемая и демонстрируемая часть личности",
			"Мистер Икс — скрытое, подсознательное",
			"Тень — то, что осознаёте, но не демонстрируете",
			"Слепое пятно — то, что видят другие, но упускаете вы",
		}
	}

	// 13. Партнёрство (7 карт)
	if strings.Contains(spread, "партнёрство") || strings.Contains(spread, "партнерство") {
		return []string{
			"Сигнификатор — что управляет отношениями",
			"Сознательный уровень спрашивающего",
			"Эмоциональный уровень спрашивающего",
			"Внешняя сторона отношений (спрашивающий)",
			"Сознательный уровень партнёра",
			"Эмоциональный уровень партнёра",
			"Внешняя сторона отношений (партнёр)",
		}
	}

	// 15. Голос Ангела-Хранителя (4 карты)
	if strings.Contains(spread, "ангел") || strings.Contains(spread, "голос") {
		return []string{
			"Истоки ситуации — ключевые события и влияния",
			"Текущее состояние дел — объективная картина",
			"Совет — конкретные дальнейшие шаги",
			"Наставление ангела-хранителя — дополнительная поддержка",
		}
	}

	// Дефолтный вариант, если расклад не распознан
	positions := make([]string, cardCount)
	for i := 0; i < cardCount; i++ {
		positions[i] = fmt.Sprintf("Позиция %d", i+1)
	}
	return positions
}

// buildLLMPrompt собирает финальный текст для LLM
func buildLLMPrompt(question, spreadName string, cards []rag.CardData) string {
	var sb strings.Builder

	sb.WriteString("Ты — опытный таролог. Проинтерпретируй расклад на основе следующих данных.\n\n")
	sb.WriteString(fmt.Sprintf("**Вопрос кверента:** %s\n", question))
	if spreadName != "" {
		sb.WriteString(fmt.Sprintf("**Используемый расклад:** %s\n\n", spreadName))
	} else {
		sb.WriteString("\n")
	}
	sb.WriteString("**Выпавшие карты и их позиции:**\n\n")

	for _, card := range cards {
		sb.WriteString(fmt.Sprintf("### %s: «%s»\n", card.Position, card.Name))
		sb.WriteString(fmt.Sprintf("- *Общее значение:* %s\n", card.Upright))
		sb.WriteString(fmt.Sprintf("- *Перевёрнутое значение:* %s\n", card.Reversed))
		sb.WriteString(fmt.Sprintf("- *Совет карты:* %s\n\n", card.Advice))
	}

	sb.WriteString("**Задача:** Дай связный, глубокий и эмпатичный ответ на вопрос кверента, опираясь на значения выпавших карт в их позициях. Не перечисляй сухие факты, а сплети их в единое повествование. Учитывай взаимосвязь между картами и их положение в раскладе.")

	return sb.String()
}

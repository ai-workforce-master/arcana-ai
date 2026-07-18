package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// --- Структуры данных ---

type Card struct {
	Name       string
	IsReversed bool
}

type CardRequest struct {
	Name       string `json:"name"`
	IsReversed bool   `json:"is_reversed"`
}

type ReadSpreadRequest struct {
	Question string        `json:"question"`
	Spread   string        `json:"spread"`
	Cards    []CardRequest `json:"cards"`
}

type ReadSpreadResponse struct {
	Interpretation string `json:"interpretation"`
}

type UserSession struct {
	Step                 string
	Question             string
	SelectedSpread       string
	SpreadPrice          int
	PaymentToken         string
	PaymentCreatedAt     time.Time
	PaymentClickTime     time.Time
	PaymentCheckUntil    time.Time
	PaymentPollingActive bool
	Deck                 []Card
}

// --- Глобальные переменные ---

var (
	sessions      = make(map[string]*UserSession)
	sessionsMutex sync.RWMutex
	maxBotToken   = getEnv("MAX_BOT_TOKEN", "")
	maxAPIBaseURL = getEnv("MAX_API_BASE_URL", "https://platform-api2.max.ru")
	backendURL    = getEnv("BACKEND_URL", "http://backend:8080")

	paymentSimDelay = make(map[string]time.Duration)
	paymentSimMutex sync.RWMutex

	insecureTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	insecureClient = &http.Client{
		Timeout:   15 * time.Second,
		Transport: insecureTransport,
	}
	pollingClient = &http.Client{
		Timeout:   35 * time.Second,
		Transport: insecureTransport,
	}
)

const (
	PaymentTokenLifetime = 1 * time.Hour
	PaymentCheckTimeout  = 10 * time.Minute
	PaymentPollInterval  = 5 * time.Second
)

var standardDeck = []string{
	"Шут", "Маг", "Верховная Жрица", "Императрица", "Император", "Иерофант", "Влюбленные", "Колесница", "Сила", "Отшельник",
	"Колесо Фортуны", "Справедливость", "Повешенный", "Смерть", "Умеренность", "Дьявол", "Башня", "Звезда", "Луна", "Солнце",
	"Суд", "Мир",
	"Туз Жезлов", "Двойка Жезлов", "Тройка Жезлов", "Четверка Жезлов", "Пятерка Жезлов", "Шестерка Жезлов", "Семерка Жезлов", "Восьмерка Жезлов", "Девятка Жезлов", "Десятка Жезлов", "Паж Жезлов", "Рыцарь Жезлов", "Королева Жезлов", "Король Жезлов",
	"Туз Кубков", "Двойка Кубков", "Тройка Кубков", "Четверка Кубков", "Пятерка Кубков", "Шестерка Кубков", "Семерка Кубков", "Восьмерка Кубков", "Девятка Кубков", "Десятка Кубков", "Паж Кубков", "Рыцарь Кубков", "Королева Кубков", "Король Кубков",
	"Туз Мечей", "Двойка Мечей", "Тройка Мечей", "Четверка Мечей", "Пятерка Мечей", "Шестерка Мечей", "Семерка Мечей", "Восьмерка Мечей", "Девятка Мечей", "Десятка Мечей", "Паж Мечей", "Рыцарь Мечей", "Королева Мечей", "Король Мечей",
	"Туз Пентаклей", "Двойка Пентаклей", "Тройка Пентаклей", "Четверка Пентаклей", "Пятерка Пентаклей", "Шестерка Пентаклей", "Семерка Пентаклей", "Восьмерка Пентаклей", "Девятка Пентаклей", "Десятка Пентаклей", "Паж Пентаклей", "Рыцарь Пентаклей", "Королева Пентаклей", "Король Пентаклей",
}

type SpreadOption struct {
	Name  string
	Price int
}

var availableSpreads = []SpreadOption{
	{"Одна карта (Карта дня)", 50},
	{"Да или Нет (3 карты)", 100},
	{"Три карты (Триплет)", 150},
	{"Крест (4 карты)", 200},
	{"Слепое пятно (4 карты)", 200},
	{"Пятикарточный расклад", 250},
	{"Голос Ангела-Хранителя (4 карты)", 250},
	{"Пирамида (6 карт)", 300},
	{"Алхимик (6 карт)", 300},
	{"Подкова (7 карт)", 300},
	{"Звезда / Чакральный (7 карт)", 300},
	{"Партнерство (7 карт)", 350},
	{"Кельтский крест (10 карт)", 500},
	{"Колесо года (12 карт)", 600},
}

// --- Вспомогательные функции ---

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func clearSession(chatID string) {
	sessionsMutex.Lock()
	delete(sessions, chatID)
	sessionsMutex.Unlock()
}

func getSession(chatID string) *UserSession {
	sessionsMutex.RLock()
	defer sessionsMutex.RUnlock()
	return sessions[chatID]
}

func setSession(chatID string, session *UserSession) {
	sessionsMutex.Lock()
	sessions[chatID] = session
	sessionsMutex.Unlock()
}

func generateDeck() []Card {
	deck := make([]Card, len(standardDeck))
	for i, name := range standardDeck {
		deck[i] = Card{
			Name:       name,
			IsReversed: mrand.Float32() < 0.3,
		}
	}
	for i := len(deck) - 1; i > 0; i-- {
		j := mrand.Intn(i + 1)
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck
}

func simulatePaymentDelay(chatID string) {
	delay := time.Duration(3+mrand.Intn(3)) * time.Second
	paymentSimMutex.Lock()
	paymentSimDelay[chatID] = delay
	paymentSimMutex.Unlock()
}

func checkPayment(chatID string, clickTime time.Time) bool {
	paymentSimMutex.RLock()
	delay, exists := paymentSimDelay[chatID]
	paymentSimMutex.RUnlock()
	if !exists {
		delay = 3 * time.Second
	}
	return time.Since(clickTime) >= delay
}

func getCardCount(spread string) int {
	s := strings.ToLower(spread)
	if strings.Contains(s, "одна") { return 1 }
	if strings.Contains(s, "да или нет") { return 3 }
	if strings.Contains(s, "три") { return 3 }
	if strings.Contains(s, "крест") && !strings.Contains(s, "кельтский") { return 4 }
	if strings.Contains(s, "слепое пятно") { return 4 }
	if strings.Contains(s, "пятикарточн") { return 5 }
	if strings.Contains(s, "ангел") || strings.Contains(s, "голос") { return 4 }
	if strings.Contains(s, "пирамида") { return 6 }
	if strings.Contains(s, "алхимик") { return 6 }
	if strings.Contains(s, "подкова") { return 7 }
	if strings.Contains(s, "звезда") || strings.Contains(s, "чакр") { return 7 }
	if strings.Contains(s, "партнерство") || strings.Contains(s, "партнёрство") { return 7 }
	if strings.Contains(s, "кельтский") { return 10 }
	if strings.Contains(s, "колесо") { return 12 }
	return 3
}

// --- MAX API Клиент ---

type maxMessageRequest struct {
	ChatID      string        `json:"chat_id"`
	Text        string        `json:"text,omitempty"`
	Attachments []interface{} `json:"attachments,omitempty"`
	Format      string        `json:"format,omitempty"`
}

func maxSendText(chatID, text string) error {
	reqBody := maxMessageRequest{ChatID: chatID, Text: text, Format: "markdown"}
	return maxAPIRequest("POST", "/messages", reqBody, nil)
}

func maxSendKeyboard(chatID, text string, buttons [][]map[string]string) error {
	attachments := []interface{}{
		map[string]interface{}{
			"type": "inline_keyboard",
			"payload": map[string]interface{}{
				"buttons": buttons,
			},
		},
	}
	reqBody := maxMessageRequest{ChatID: chatID, Text: text, Attachments: attachments, Format: "markdown"}
	return maxAPIRequest("POST", "/messages", reqBody, nil)
}

func maxAnswerCallback(callbackQueryID, text string) error {
	reqBody := map[string]string{
		"callback_query_id": callbackQueryID,
		"text":              text,
	}
	return maxAPIRequest("POST", "/answer_callback", reqBody, nil)
}

func maxAPIRequest(method, path string, reqBody interface{}, respBody interface{}) error {
	url := maxAPIBaseURL + path
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", maxBotToken)

	resp, err := insecureClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MAX API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	if respBody != nil {
		return json.NewDecoder(resp.Body).Decode(respBody)
	}
	return nil
}

// --- Логика бота ---

func runPaymentPoller(chatID string, token string, clickTime time.Time) {
	ticker := time.NewTicker(PaymentPollInterval)
	defer ticker.Stop()
	timeout := time.After(PaymentCheckTimeout)

	for {
		select {
		case <-ticker.C:
			session := getSession(chatID)
			if session == nil || session.PaymentToken != token {
				return
			}
			if checkPayment(chatID, clickTime) {
				maxSendText(chatID, "✅ Оплата подтверждена! Начинаем ритуал...")
				go startShuffleAndReading(chatID)
				return
			}
		case <-timeout:
			session := getSession(chatID)
			if session != nil && session.PaymentToken == token {
				remaining := PaymentTokenLifetime - time.Since(session.PaymentCreatedAt)
				var msg string
				if remaining > 0 {
					msg = fmt.Sprintf("⏳ Оплата не подтверждена за 10 минут. Токен действителен еще %d мин. Нажмите '✅ Я оплатил' снова или начните с /start", int(remaining.Minutes()))
				} else {
					msg = "⏳ Токен истёк. Начните заново с /start"
					clearSession(chatID)
				}
				maxSendText(chatID, msg)
				session.PaymentPollingActive = false
				setSession(chatID, session)
			}
			return
		}
	}
}

func startShuffleAndReading(chatID string) {
	session := getSession(chatID)
	if session == nil {
		return
	}

	session.Deck = generateDeck()
	session.Step = "shuffling"
	setSession(chatID, session)

	maxSendText(chatID, "🌪️ Перемешиваем колоду...")
	time.Sleep(1500 * time.Millisecond)
	maxSendText(chatID, "🃏 Карты выравниваются и заряжаются вашим намерением...")
	time.Sleep(1500 * time.Millisecond)

	buttons := [][]map[string]string{
		{{"text": "✂️ Срезать колоду", "payload": "cut_deck"}},
	}
	maxSendKeyboard(chatID, "✨ Колода готова! Нажмите кнопку ниже, чтобы срезать колоду (это случайным образом разделит её и переместит верхнюю часть вниз).", buttons)
}

func handleUpdate(update map[string]interface{}) {
	payload, ok := update["payload"].(map[string]interface{})
	if !ok {
		return
	}

	var chatID, text, callbackQueryID, callbackData string
	var isCallback bool

	// MAX API предоставляет chat_id в корне объекта update (как число float64 или строку)
	if cid, ok := update["chat_id"].(float64); ok {
		chatID = fmt.Sprintf("%.0f", cid)
	} else if cid, ok := update["chat_id"].(string); ok {
		chatID = cid
	}

	if msg, ok := payload["message"].(map[string]interface{}); ok {
		if body, ok := msg["body"].(map[string]interface{}); ok {
			if t, ok := body["text"].(string); ok {
				text = strings.TrimSpace(t)
			}
		}
	} else if cb, ok := payload["message_callback"].(map[string]interface{}); ok {
		isCallback = true
		if id, ok := cb["id"].(float64); ok {
			callbackQueryID = fmt.Sprintf("%.0f", id)
		} else if id, ok := cb["id"].(string); ok {
			callbackQueryID = id
		}
		if data, ok := cb["payload"].(string); ok {
			callbackData = data
		}
		// Fallback для chatID, если его нет в корне (маловероятно, но для надежности)
		if chatID == "" {
			if msg, ok := cb["message"].(map[string]interface{}); ok {
				if sender, ok := msg["sender"].(map[string]interface{}); ok {
					if uid, ok := sender["user_id"].(float64); ok {
						chatID = fmt.Sprintf("%.0f", uid)
					} else if uid, ok := sender["user_id"].(string); ok {
						chatID = uid
					}
				}
			}
		}
	}

	if chatID == "" {
		return
	}

	session := getSession(chatID)

	// 1. Старт
	if text == "/start" || text == "🔮 Новый расклад" {
		clearSession(chatID)
		setSession(chatID, &UserSession{Step: "awaiting_question"})
		maxSendText(chatID, "🔮 Добро пожаловать в Arcana AI!\n\nЗадайте ваш вопрос картам Таро (например: 'Что меня ждет в новом проекте?'):")
		return
	}

	// 2. Вопрос
	if session != nil && session.Step == "awaiting_question" && text != "" {
		session.Question = text
		session.Step = "awaiting_spread"
		setSession(chatID, session)

		var buttons [][]map[string]string
		for _, sp := range availableSpreads {
			buttons = append(buttons, []map[string]string{
				{"text": fmt.Sprintf("%s — %d₽", sp.Name, sp.Price), "payload": fmt.Sprintf("select_spread|%s|%d", sp.Name, sp.Price)},
			})
		}
		maxSendKeyboard(chatID, "Отличный вопрос! Выберите тип расклада:", buttons)
		return
	}

	// 3. Выбор расклада
	if isCallback && strings.HasPrefix(callbackData, "select_spread|") {
		parts := strings.Split(callbackData, "|")
		if len(parts) == 3 {
			session.SelectedSpread = parts[1]
			fmt.Sscanf(parts[2], "%d", &session.SpreadPrice)
			session.PaymentToken = uuid.New().String()
			session.PaymentCreatedAt = time.Now()
			session.Step = "awaiting_payment"
			setSession(chatID, session)
			simulatePaymentDelay(chatID)

			maxAnswerCallback(callbackQueryID, "Расклад выбран")

			msg := fmt.Sprintf("Вы выбрали: *%s*\n💰 Стоимость: *%d₽*\n\n📋 Номер платежа: %s\n⏰ Токен действителен 1 час\n\nПереведите сумму на карту: 2200 0000 0000 0000\n\nПосле перевода нажмите кнопку ниже.", session.SelectedSpread, session.SpreadPrice, session.PaymentToken)

			buttons := [][]map[string]string{
				{{"text": "✅ Я оплатил", "payload": "check_payment"}},
			}
			maxSendKeyboard(chatID, msg, buttons)
		}
		return
	}

	// 4. Проверка оплаты
	if isCallback && callbackData == "check_payment" {
		if time.Since(session.PaymentCreatedAt) > PaymentTokenLifetime {
			maxAnswerCallback(callbackQueryID, "Токен истёк")
			maxSendText(chatID, "⏳ Токен истёк. Начните заново с /start")
			clearSession(chatID)
			return
		}
		if session.PaymentPollingActive {
			maxAnswerCallback(callbackQueryID, "Оплата уже проверяется")
			return
		}

		session.PaymentClickTime = time.Now()
		session.PaymentPollingActive = true
		setSession(chatID, session)

		maxAnswerCallback(callbackQueryID, "Проверяем оплату...")
		maxSendText(chatID, "⏳ Проверяем оплату в системе... Это займет несколько секунд.")
		go runPaymentPoller(chatID, session.PaymentToken, session.PaymentClickTime)
		return
	}

	// 5. Срез колоды и интерпретация
	if isCallback && callbackData == "cut_deck" {
		maxAnswerCallback(callbackQueryID, "Колода срезана!")

		selectedSpread := session.SelectedSpread
		question := session.Question
		deck := session.Deck

		cutIndex := mrand.Intn(len(deck))
		deck = append(deck[cutIndex:], deck[:cutIndex]...)

		maxSendText(chatID, "🔮 Выкладываем карты на стол...")

		cardCount := getCardCount(selectedSpread)
		var selectedCards []CardRequest
		var cardTexts []string

		for i := 0; i < cardCount && i < len(deck); i++ {
			card := deck[i]
			selectedCards = append(selectedCards, CardRequest{Name: card.Name, IsReversed: card.IsReversed})

			reversedStr := ""
			if card.IsReversed {
				reversedStr = " 🔄 (Перевёрнутая)"
			}
			cardTexts = append(cardTexts, fmt.Sprintf("🃏 Позиция %d: %s%s", i+1, card.Name, reversedStr))
		}

		maxSendText(chatID, "Выпавшие карты:\n\n"+strings.Join(cardTexts, "\n"))
		maxSendText(chatID, "⏳ Запрашиваем мудрость ИИ для интерпретации этого расклада...")

		reqBody := ReadSpreadRequest{Question: question, Spread: selectedSpread, Cards: selectedCards}
		jsonData, _ := json.Marshal(reqBody)
		
		resp, err := insecureClient.Post(backendURL+"/api/read-spread", "application/json", bytes.NewBuffer(jsonData))
		clearSession(chatID)

		if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
			maxSendText(chatID, "❌ Произошла ошибка при обращении к оракулу. Попробуйте позже.")
			return
		}

		var result ReadSpreadResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			maxSendText(chatID, "❌ Ошибка при обработке ответа от ИИ.")
			return
		}

		answerText := fmt.Sprintf("🔮 Расклад: %s\n📜 Вопрос: %s\n\n%s\n\nХотите задать еще один вопрос? Напишите /start", selectedSpread, question, result.Interpretation)
		maxSendText(chatID, answerText)
		return
	}

	// Fallback
	if session == nil {
		maxSendText(chatID, "Пожалуйста, напишите /start или '🔮 Новый расклад' для начала.")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	if maxBotToken == "" {
		log.Fatal("MAX_BOT_TOKEN is not set")
	}

	log.Printf("Starting MAX Bot...")
	log.Printf("MAX_API_BASE_URL: %s", maxAPIBaseURL)
	log.Printf("MAX_BOT_TOKEN: %s...", maxBotToken[:min(10, len(maxBotToken))])

	for {
		url := fmt.Sprintf("%s/updates", maxAPIBaseURL)
		log.Printf("Polling URL: %s", url)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", maxBotToken)
		req.Header.Set("Accept", "application/json")

		resp, err := pollingClient.Do(req)
		if err != nil {
			log.Printf("Error polling updates: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		log.Printf("Response status: %d", resp.StatusCode)
		log.Printf("Response body (first 500 chars): %s", string(bodyBytes[:min(500, len(bodyBytes))]))

		// ИСПРАВЛЕНО: Правильная структура ответа MAX API
		var maxResponse struct {
			Updates []map[string]interface{} `json:"updates"`
			Marker  int64                    `json:"marker"`
		}

		if err := json.Unmarshal(bodyBytes, &maxResponse); err != nil {
			log.Printf("Error decoding updates: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, update := range maxResponse.Updates {
			go handleUpdate(update)
		}
		
		time.Sleep(1 * time.Second)
	}
}

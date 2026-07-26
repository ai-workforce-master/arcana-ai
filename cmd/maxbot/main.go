package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"net"
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

type CardInterpretation struct {
	Card           CardRequest `json:"card"`
	Interpretation string      `json:"interpretation"`
}

type ReadSpreadRequest struct {
	Question string        `json:"question"`
	Spread   string        `json:"spread"`
	Cards    []CardRequest `json:"cards"`
}

type ReadSpreadResponse struct {
	Intro      string               `json:"intro"`
	Cards      []CardInterpretation `json:"cards"`
	Conclusion string               `json:"conclusion"`
}

type UserSession struct {
	Step             string
	Question         string
	SelectedSpread   string
	SpreadPrice      int
	PaymentID        string
	PaymentCreatedAt time.Time
	Deck             []Card
}

// --- Глобальные переменные ---

var (
	sessions      = make(map[string]*UserSession)
	sessionsMutex sync.RWMutex
	maxBotToken   = getEnv("MAX_BOT_TOKEN", "")
	maxAPIBaseURL = getEnv("MAX_API_BASE_URL", "https://platform-api2.max.ru")
	backendURL    = getEnv("BACKEND_URL", "http://backend:8080")

	workingMaxIP = "155.212.204.75"

	cardTokens = make(map[string]string)

	customTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, "tcp4", addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "platform-api2.max.ru",
		},
		TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		DisableKeepAlives: true,
	}

	pollingClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: customTransport,
	}

	insecureClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: customTransport,
	}
)

const (
	PaymentTokenLifetime = 1 * time.Hour
)

var standardDeck = []string{
	"Шут", "Маг", "Верховная Жрица", "Императрица", "Император", "Иерофант", "Влюбленные", "Колесница", "Сила", "Отшельник",
	"Колесо Фортуны", "Справедливость", "Повешенный", "Смерть", "Умеренность", "Дьявол", "Башня", "Звезда", "Луна", "Солнце",
	"Суд", "Мир",
	"Туз Кубков", "Двойка Кубков", "Тройка Кубков", "Четверка Кубков", "Пятерка Кубков", "Шестерка Кубков", "Семерка Кубков", "Восьмерка Кубков", "Девятка Кубков", "Десятка Кубков", "Паж Кубков", "Рыцарь Кубков", "Королева Кубков", "Король Кубков",
	"Туз Пентаклей", "Двойка Пентаклей", "Тройка Пентаклей", "Четверка Пентаклей", "Пятерка Пентаклей", "Шестерка Пентаклей", "Семерка Пентаклей", "Восьмерка Пентаклей", "Девятка Пентаклей", "Десятка Пентаклей", "Паж Пентаклей", "Рыцарь Пентаклей", "Королева Пентаклей", "Король Пентаклей",
	"Туз Мечей", "Двойка Мечей", "Тройка Мечей", "Четверка Мечей", "Пятерка Мечей", "Шестерка Мечей", "Семерка Мечей", "Восьмерка Мечей", "Девятка Мечей", "Десятка Мечей", "Паж Мечей", "Рыцарь Мечей", "Королева Мечей", "Король Мечей",
	"Туз Жезлов", "Двойка Жезлов", "Тройка Жезлов", "Четверка Жезлов", "Пятерка Жезлов", "Шестерка Жезлов", "Семерка Жезлов", "Восьмерка Жезлов", "Девятка Жезлов", "Десятка Жезлов", "Паж Жезлов", "Рыцарь Жезлов", "Королева Жезлов", "Король Жезлов",
}

type SpreadOption struct {
	Name  string
	Price int
}

var availableSpreads = []SpreadOption{
	{"Одна карта (Карта дня)", 29},
	{"Да или Нет (3 карты)", 69},
	{"Три карты (Триплет)", 69},
	{"Крест (4 карты)", 89},
	{"Слепое пятно (4 карты)", 89},
	{"Пятикарточный расклад", 109},
	{"Голос Ангела-Хранителя (4 карты)", 89},
	{"Пирамида (6 карт)", 129},
	{"Алхимик (6 карт)", 129},
	{"Подкова (7 карт)", 149},
	{"Звезда / Чакральный (7 карт)", 149},
	{"Партнерство (7 карт)", 149},
	{"Кельтский крест (10 карт)", 199},
	{"Колесо года (12 карт)", 229},
}

// --- Вспомогательные функции ---

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func loadCardTokens() {
	data, err := os.ReadFile("cards_tokens.json")
	if err != nil {
		log.Printf("WARNING: Could not load cards_tokens.json: %v. Images will not be sent.", err)
		return
	}
	if err := json.Unmarshal(data, &cardTokens); err != nil {
		log.Printf("WARNING: Could not parse cards_tokens.json: %v", err)
		return
	}
	log.Printf("SUCCESS: Loaded %d card tokens from cards_tokens.json", len(cardTokens))
}

func clearSession(userID string) {
	sessionsMutex.Lock()
	delete(sessions, userID)
	sessionsMutex.Unlock()
}

func getSession(userID string) *UserSession {
	sessionsMutex.RLock()
	defer sessionsMutex.RUnlock()
	return sessions[userID]
}

func setSession(userID string, session *UserSession) {
	sessionsMutex.Lock()
	sessions[userID] = session
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

func getCardNumber(cardName string) int {
	cardMap := map[string]int{
		"Шут": 0, "Маг": 1, "Верховная Жрица": 2, "Императрица": 3, 
		"Император": 4, "Иерофант": 5, "Влюбленные": 6, "Колесница": 7, 
		"Сила": 8, "Отшельник": 9, "Колесо Фортуны": 10, "Справедливость": 11, 
		"Повешенный": 12, "Смерть": 13, "Умеренность": 14, "Дьявол": 15, 
		"Башня": 16, "Звезда": 17, "Луна": 18, "Солнце": 19, "Суд": 20, "Мир": 21,
		"Туз Кубков": 22, "Двойка Кубков": 23, "Тройка Кубков": 24, 
		"Четверка Кубков": 25, "Пятерка Кубков": 26, "Шестерка Кубков": 27, 
		"Семерка Кубков": 28, "Восьмерка Кубков": 29, "Девятка Кубков": 30, 
		"Десятка Кубков": 31, "Паж Кубков": 32, "Рыцарь Кубков": 33, 
		"Королева Кубков": 34, "Король Кубков": 35,
		"Туз Пентаклей": 36, "Двойка Пентаклей": 37, "Тройка Пентаклей": 38, 
		"Четверка Пентаклей": 39, "Пятерка Пентаклей": 40, "Шестерка Пентаклей": 41, 
		"Семерка Пентаклей": 42, "Восьмерка Пентаклей": 43, "Девятка Пентаклей": 44, 
		"Десятка Пентаклей": 45, "Паж Пентаклей": 46, "Рыцарь Пентаклей": 47, 
		"Королева Пентаклей": 48, "Король Пентаклей": 49,
		"Туз Мечей": 50, "Двойка Мечей": 51, "Тройка Мечей": 52, 
		"Четверка Мечей": 53, "Пятерка Мечей": 54, "Шестерка Мечей": 55, 
		"Семерка Мечей": 56, "Восьмерка Мечей": 57, "Девятка Мечей": 58, 
		"Десятка Мечей": 59, "Паж Мечей": 60, "Рыцарь Мечей": 61, 
		"Королева Мечей": 62, "Король Мечей": 63,
		"Туз Жезлов": 64, "Двойка Жезлов": 65, "Тройка Жезлов": 66, 
		"Четверка Жезлов": 67, "Пятерка Жезлов": 68, "Шестерка Жезлов": 69, 
		"Семерка Жезлов": 70, "Восьмерка Жезлов": 71, "Девятка Жезлов": 72, 
		"Десятка Жезлов": 73, "Паж Жезлов": 74, "Рыцарь Жезлов": 75, 
		"Королева Жезлов": 76, "Король Жезлов": 77,
	}
	if number, exists := cardMap[cardName]; exists {
		return number
	}
	return -1
}

// --- MAX API Клиент ---

func maxSendText(userID int64, text string) error {
	reqBody := map[string]interface{}{
		"text":   text,
		"format": "markdown",
	}
	return maxAPIRequest("POST", fmt.Sprintf("/messages?user_id=%d", userID), reqBody, nil)
}

func maxSendKeyboard(userID int64, text string, buttons [][]map[string]interface{}) error {
	attachments := []interface{}{
		map[string]interface{}{
			"type": "inline_keyboard",
			"payload": map[string]interface{}{
				"buttons": buttons,
			},
		},
	}
	reqBody := map[string]interface{}{
		"text":        text,
		"attachments": attachments,
		"format":      "markdown",
	}
	return maxAPIRequest("POST", fmt.Sprintf("/messages?user_id=%d", userID), reqBody, nil)
}

func maxSendPhotoByToken(userID int64, text, token string) error {
	attachments := []interface{}{
		map[string]interface{}{
			"type": "image",
			"payload": map[string]interface{}{
				"token": token,
			},
		},
	}
	reqBody := map[string]interface{}{
		"text":        text,
		"attachments": attachments,
		"format":      "markdown",
	}
	return maxAPIRequest("POST", fmt.Sprintf("/messages?user_id=%d", userID), reqBody, nil)
}

func maxAnswerCallback(callbackQueryID, notificationText string) error {
	reqBody := map[string]interface{}{
		"notification": notificationText,
	}
	return maxAPIRequest("POST", fmt.Sprintf("/answers?callback_id=%s", callbackQueryID), reqBody, nil)
}

func maxAPIRequest(method, path string, reqBody interface{}, respBody interface{}) error {
	url := "https://" + workingMaxIP + path

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
	req.Host = "platform-api2.max.ru"

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

func sendSpreadsKeyboard(userID int64) {
	var buttons [][]map[string]interface{}
	for _, sp := range availableSpreads {
		buttons = append(buttons, []map[string]interface{}{
			{"type": "callback", "text": fmt.Sprintf("%s — %d₽", sp.Name, sp.Price), "payload": fmt.Sprintf("select_spread|%s|%d", sp.Name, sp.Price)},
		})
	}
	maxSendKeyboard(userID, "Выберите тип расклада:", buttons)
}

func startShuffleAndReading(userID int64, userStr string) {
	session := getSession(userStr)
	if session == nil { return }

	session.Deck = generateDeck()
	session.Step = "shuffling"
	setSession(userStr, session)

	maxSendText(userID, "🌪️ Перемешиваем колоду...")
	time.Sleep(1500 * time.Millisecond)
	maxSendText(userID, "🃏 Карты выравниваются и заряжаются вашим намерением...")
	time.Sleep(1500 * time.Millisecond)

	buttons := [][]map[string]interface{}{
		{
			{
				"type":    "callback",
				"text":    "✂️ Срезать колоду",
				"payload": "cut_deck",
			},
		},
	}
	maxSendKeyboard(userID, "✨ Колода готова! Нажмите кнопку ниже, чтобы срезать колоду.", buttons)
}

// --- Логика бота ---

func handleUpdate(update map[string]interface{}) {
	var userID int64
	var text, callbackQueryID, callbackData string
	var isCallback bool

	updateType, _ := update["update_type"].(string)

	if updateType == "message_created" {
		if msg, ok := update["message"].(map[string]interface{}); ok {
			if sender, ok := msg["sender"].(map[string]interface{}); ok {
				if uid, ok := sender["user_id"].(float64); ok {
					userID = int64(uid)
				}
			}

			if body, ok := msg["body"].(map[string]interface{}); ok {
				if t, ok := body["text"].(string); ok {
					text = strings.TrimSpace(t)
				}
			}
		}
	} else if updateType == "message_callback" || update["callback"] != nil {
		isCallback = true
		if cb, ok := update["callback"].(map[string]interface{}); ok {
			if id, ok := cb["callback_id"].(string); ok {
				callbackQueryID = id
			} else if id, ok := cb["callback_id"].(float64); ok {
				callbackQueryID = fmt.Sprintf("%.0f", id)
			}
			
			if data, ok := cb["payload"].(string); ok {
				callbackData = data
			}
			
			if user, ok := cb["user"].(map[string]interface{}); ok {
				if uid, ok := user["user_id"].(float64); ok {
					userID = int64(uid)
				}
			}
		}
	}

	if userID == 0 {
		return
	}

	userStr := fmt.Sprintf("%d", userID)
	session := getSession(userStr)

	if text == "/start" || text == "🔮 Новый расклад" {
		clearSession(userStr)
		setSession(userStr, &UserSession{Step: "awaiting_question"})
		maxSendText(userID, "🔮 Добро пожаловать в Arcana AI!\n\nЗадайте ваш вопрос картам Таро (например: 'Что меня ждет в новом проекте?'):")
		return
	}

	if session != nil && session.Step == "awaiting_question" && text != "" {
		session.Question = text
		session.Step = "awaiting_spread"
		setSession(userStr, session)
		sendSpreadsKeyboard(userID)
		return
	}

	// 3. Выбор расклада и создание платежа
	if isCallback && strings.HasPrefix(callbackData, "select_spread|") {
		parts := strings.Split(callbackData, "|")
		if len(parts) == 3 {
			if session == nil {
				session = &UserSession{}
			}
			session.SelectedSpread = parts[1]
			fmt.Sscanf(parts[2], "%d", &session.SpreadPrice)
			session.Step = "awaiting_payment"
			setSession(userStr, session)

			maxAnswerCallback(callbackQueryID, "Расклад выбран")
			maxSendText(userID, "⏳ Создаем ссылку на оплату...")

			sessionID := uuid.New().String()
			reqBody := map[string]interface{}{
				"session_id":  sessionID,
				"spread_name": session.SelectedSpread,
				"amount":      float64(session.SpreadPrice),
			}
			jsonData, _ := json.Marshal(reqBody)
			
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Post(backendURL+"/api/create-payment", "application/json", bytes.NewBuffer(jsonData))
			
			if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
				log.Printf("ERROR MAX: Failed to create payment for user %d: err=%v, resp=%v", userID, err, resp)
				maxSendText(userID, "❌ Не удалось создать платеж. Попробуйте позже.")
				clearSession(userStr)
				return
			}
			
			var payResp struct {
				PaymentID       string `json:"payment_id"`
				ConfirmationURL string `json:"confirmation_url"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&payResp); err != nil {
				log.Printf("ERROR MAX: Failed to decode payment response for user %d: %v", userID, err)
				maxSendText(userID, "❌ Ошибка при обработке ответа от платежной системы.")
				clearSession(userStr)
				return
			}
			
			session.PaymentID = payResp.PaymentID
			session.PaymentCreatedAt = time.Now()
			setSession(userStr, session)

			msg := fmt.Sprintf("Вы выбрали: *%s*\n💰 Стоимость: *%d₽*\n\nНажмите кнопку ниже для оплаты через ЮKassa.\nПосле оплаты нажмите 'Проверить оплату'.", session.SelectedSpread, session.SpreadPrice)
			
			buttons := [][]map[string]interface{}{
				{{"type": "open_link", "text": "💳 Оплатить расклад", "link": payResp.ConfirmationURL}},
				{{"type": "callback", "text": "🔄 Проверить оплату", "payload": "check_payment"}},
			}

			maxSendKeyboard(userID, msg, buttons)
		}
		return
	}

	// 4. Проверка оплаты
	if isCallback && callbackData == "check_payment" {
		if session == nil || session.PaymentID == "" {
			maxAnswerCallback(callbackQueryID, "Сессия не найдена")
			maxSendText(userID, "⏳ Сессия не найдена. Начните заново с /start")
			return
		}
		if time.Since(session.PaymentCreatedAt) > PaymentTokenLifetime {
			maxAnswerCallback(callbackQueryID, "Токен истёк")
			maxSendText(userID, " Время на оплату истекло. Начните заново с /start")
			clearSession(userStr)
			return
		}

		maxAnswerCallback(callbackQueryID, "Проверяем...")
		maxSendText(userID, "⏳ Проверяем статус оплаты...")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(backendURL + "/api/payment-status/" + session.PaymentID)

		if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
			maxSendText(userID, "️ Не удалось проверить статус. Попробуйте позже.")
			return
		}

		var statusResp struct { Status string `json:"status"` }
		json.NewDecoder(resp.Body).Decode(&statusResp)

		if statusResp.Status == "succeeded" {
			maxSendText(userID, "✅ Оплата подтверждена! Начинаем ритуал...")
			go startShuffleAndReading(userID, userStr)
		} else {
			maxSendText(userID, " Оплата ещё не прошла. Выберите расклад снова:")
			sendSpreadsKeyboard(userID)
		}
		return
	}

	// 5. Срез колоды и РАСКЛАД
	if isCallback && callbackData == "cut_deck" {
		maxAnswerCallback(callbackQueryID, "Колода срезана!")

		if session == nil { return }

		selectedSpread := session.SelectedSpread
		question := session.Question
		deck := session.Deck

		cutIndex := mrand.Intn(len(deck))
		deck = append(deck[cutIndex:], deck[:cutIndex]...)

		maxSendText(userID, "🔮 Выкладываем карты на стол...")

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
			cardTexts = append(cardTexts, fmt.Sprintf(" Позиция %d: %s%s", i+1, card.Name, reversedStr))
		}

		maxSendText(userID, "Выпавшие карты:\n\n"+strings.Join(cardTexts, "\n"))
		maxSendText(userID, "⏳ Запрашиваем мудрость ИИ...")

		reqBody := ReadSpreadRequest{Question: question, Spread: selectedSpread, Cards: selectedCards}
		jsonData, _ := json.Marshal(reqBody)
		targetURL := backendURL + "/api/read-spread"

		resp, err := insecureClient.Post(targetURL, "application/json", bytes.NewBuffer(jsonData))
		clearSession(userStr)

		if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
			maxSendText(userID, "❌ Ошибка при обращении к оракулу.")
			return
		}

		var result ReadSpreadResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			maxSendText(userID, " Ошибка при обработке ответа от ИИ.")
			return
		}

		if strings.TrimSpace(result.Intro) != "" {
			maxSendText(userID, result.Intro)
			time.Sleep(1000 * time.Millisecond)
		}

		for i, cardInterp := range result.Cards {
			cardName := cardInterp.Card.Name
			isReversed := cardInterp.Card.IsReversed
			
			positionText := ""
			if len(result.Cards) > 1 {
				positionText = fmt.Sprintf("**Позиция %d**\n", i+1)
			}
			
			reversedNote := ""
			if isReversed {
				reversedNote = " 🔄 *(Перевёрнутая)*"
			}
			
			cardText := fmt.Sprintf("%s**%s**%s\n\n%s", positionText, cardName, reversedNote, cardInterp.Interpretation)
			
			cardNumber := getCardNumber(cardName)
			var photoToken string
			if cardNumber >= 0 {
				filename := fmt.Sprintf("RWS_%05d.jpg", cardNumber)
				photoToken = cardTokens[filename]
			}
			
			if photoToken != "" {
				err := maxSendPhotoByToken(userID, cardText, photoToken)
				if err != nil {
					maxSendText(userID, cardText)
				}
			} else {
				maxSendText(userID, cardText)
			}
			
			if i < len(result.Cards)-1 {
				time.Sleep(1500 * time.Millisecond)
			}
		}

		conclusion := strings.TrimSpace(result.Conclusion)
		if conclusion != "" {
			maxSendText(userID, conclusion)
			time.Sleep(500 * time.Millisecond)
		}

		maxSendText(userID, "✨ Если у вас есть ещё вопросы, просто напишите /start, и мы проведём новый расклад!")
		return
	}

	if session == nil && text != "" {
		maxSendText(userID, "Пожалуйста, напишите /start или '🔮 Новый расклад' для начала.")
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

	loadCardTokens()

	log.Printf("Starting MAX Bot...")
	log.Printf("MAX_API_BASE_URL: %s", maxAPIBaseURL)
	log.Printf("MAX_BOT_TOKEN: %s...", maxBotToken[:min(10, len(maxBotToken))])

	for {
		url := "https://" + workingMaxIP + "/updates"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", maxBotToken)
		req.Header.Set("Accept", "application/json")
		req.Host = "platform-api2.max.ru"

		resp, err := pollingClient.Do(req)
		if err != nil {
			log.Printf("Error polling: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

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

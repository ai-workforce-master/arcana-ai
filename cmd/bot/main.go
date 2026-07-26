package main

import (
	"bytes"
	"context"
	"crypto/rand"
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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	Step                 string
	Question             string
	SelectedSpread       string
	SpreadPrice          int
	PaymentID            string // Реальный ID платежа из бэкенда
	PaymentCreatedAt     time.Time
	PaymentPollingActive bool
	Deck                 []Card
}

// --- Глобальные переменные ---

var (
	sessions      = make(map[int64]*UserSession)
	sessionsMutex sync.RWMutex
	botToken      = getEnv("TELEGRAM_BOT_TOKEN", "")
	backendURL    = getEnv("BACKEND_URL", "http://127.0.0.1:18080")

	cardNameToNumber = map[string]int{
		"Шут": 0, "Маг": 1, "Верховная Жрица": 2, "Императрица": 3, "Император": 4, "Иерофант": 5, "Влюбленные": 6, "Колесница": 7, "Сила": 8, "Отшельник": 9, "Колесо Фортуны": 10, "Справедливость": 11, "Повешенный": 12, "Смерть": 13, "Умеренность": 14, "Дьявол": 15, "Башня": 16, "Звезда": 17, "Луна": 18, "Солнце": 19, "Суд": 20, "Мир": 21,
		"Туз Кубков": 22, "Двойка Кубков": 23, "Тройка Кубков": 24, "Четверка Кубков": 25, "Пятерка Кубков": 26, "Шестерка Кубков": 27, "Семерка Кубков": 28, "Восьмерка Кубков": 29, "Девятка Кубков": 30, "Десятка Кубков": 31, "Паж Кубков": 32, "Рыцарь Кубков": 33, "Королева Кубков": 34, "Король Кубков": 35,
		"Туз Пентаклей": 36, "Двойка Пентаклей": 37, "Тройка Пентаклей": 38, "Четверка Пентаклей": 39, "Пятерка Пентаклей": 40, "Шестерка Пентаклей": 41, "Семерка Пентаклей": 42, "Восьмерка Пентаклей": 43, "Девятка Пентаклей": 44, "Десятка Пентаклей": 45, "Паж Пентаклей": 46, "Рыцарь Пентаклей": 47, "Королева Пентаклей": 48, "Король Пентаклей": 49,
		"Туз Мечей": 50, "Двойка Мечей": 51, "Тройка Мечей": 52, "Четверка Мечей": 53, "Пятерка Мечей": 54, "Шестерка Мечей": 55, "Семерка Мечей": 56, "Восьмерка Мечей": 57, "Девятка Мечей": 58, "Десятка Мечей": 59, "Паж Мечей": 60, "Рыцарь Мечей": 61, "Королева Мечей": 62, "Король Мечей": 63,
		"Туз Жезлов": 64, "Двойка Жезлов": 65, "Тройка Жезлов": 66, "Четверка Жезлов": 67, "Пятерка Жезлов": 68, "Шестерка Жезлов": 69, "Семерка Жезлов": 70, "Восьмерка Жезлов": 71, "Девятка Жезлов": 72, "Десятка Жезлов": 73, "Паж Жезлов": 74, "Рыцарь Жезлов": 75, "Королева Жезлов": 76, "Король Жезлов": 77,
	}
)

const PaymentTokenLifetime = 1 * time.Hour

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
	ID    string
	Name  string
	Price int
}

var availableSpreads = []SpreadOption{
	{"one-card", "Одна карта (Карта дня)", 29},
	{"yes-no", "Да или Нет (3 карты)", 69},
	{"triplet", "Три карты (Триплет)", 69},
	{"cross", "Крест (4 карты)", 89},
	{"blind-spot", "Слепое пятно (4 карты)", 89},
	{"guardian-angel", "Голос Ангела-Хранителя (4 карты)", 89},
	{"five-cards", "Пятикарточный расклад", 109},
	{"pyramid", "Пирамида (6 карт)", 129},
	{"alchemist", "Алхимик (6 карт)", 129},
	{"horseshoe", "Подкова (7 карт)", 149},
	{"star", "Звезда / Чакральный (7 карт)", 149},
	{"partnership", "Партнерство (7 карт)", 149},
	{"celtic-cross", "Кельтский крест (10 карт)", 199},
	{"wheel-of-year", "Колесо года (12 карт)", 229},
}

// --- Вспомогательные функции ---

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func clearSession(chatID int64) {
	sessionsMutex.Lock()
	delete(sessions, chatID)
	sessionsMutex.Unlock()
}

func getSession(chatID int64) *UserSession {
	sessionsMutex.RLock()
	defer sessionsMutex.RUnlock()
	return sessions[chatID]
}

func setSession(chatID int64, session *UserSession) {
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

func generatePaymentToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
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

func getCardImageURL(cardName string) string {
	num, exists := cardNameToNumber[cardName]
	if !exists {
		return "https://ai-arcana.ru/cards/RWS_00000.jpg"
	}
	return fmt.Sprintf("https://ai-arcana.ru/cards/RWS_%05d.jpg", num)
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

// --- Проверка статуса оплаты через бэкенд ---

func checkPaymentStatus(bot *tgbotapi.BotAPI, chatID int64, paymentID string) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(backendURL + "/api/payment-status/" + paymentID)
	
	session := getSession(chatID)
	if session == nil {
		return
	}
	
	if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
		log.Printf("ERROR TG: Failed to check payment status for chat %d: err=%v, resp=%v", chatID, err, resp)
		bot.Send(tgbotapi.NewMessage(chatID, "⚠️ Не удалось проверить статус оплаты. Попробуйте нажать '✅ Я оплатил' еще раз через несколько секунд."))
		session.PaymentPollingActive = false
		setSession(chatID, session)
		return
	}
	
	var statusResp struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		log.Printf("ERROR TG: Failed to decode payment status for chat %d: %v", chatID, err)
		bot.Send(tgbotapi.NewMessage(chatID, "⚠️ Ошибка при обработке статуса оплаты."))
		session.PaymentPollingActive = false
		setSession(chatID, session)
		return
	}
	
	if statusResp.Status == "succeeded" {
		bot.Send(tgbotapi.NewMessage(chatID, "✅ Оплата подтверждена! Начинаем ритуал..."))
		go startShuffleAndReading(bot, chatID)
	} else {
		bot.Send(tgbotapi.NewMessage(chatID, "⏳ Оплата ещё не подтверждена. Пожалуйста, завершите оплату и нажмите кнопку снова."))
		session.PaymentPollingActive = false
		setSession(chatID, session)
	}
}

func startShuffleAndReading(bot *tgbotapi.BotAPI, chatID int64) {
	session := getSession(chatID)
	if session == nil {
		return
	}

	session.Deck = generateDeck()
	session.Step = "shuffling"
	setSession(chatID, session)

	animMsg := tgbotapi.NewMessage(chatID, "🌪️ *Перемешиваем колоду...*")
	animMsg.ParseMode = "Markdown"
	sentMsg, _ := bot.Send(animMsg)

	time.Sleep(1200 * time.Millisecond)
	bot.Request(tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "🃏 *Карты выравниваются и заряжаются вашим намерением...*"))
	
	time.Sleep(1200 * time.Millisecond)
	bot.Request(tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "✨ *Колода готова к раскрытию тайн.*"))
	
	time.Sleep(800 * time.Millisecond)

	msg := tgbotapi.NewMessage(chatID, "Чтобы сделать расклад максимально личным, предложите колоде выбрать момент.\n\nНажмите кнопку ниже, чтобы **срезать колоду**.")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✂️ Срезать колоду", "cut_deck"),
		),
	)
	bot.Send(msg)
}

// --- Основная логика бота ---

func main() {
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is not set")
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", addr)
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Timeout:   120 * time.Second,
		Transport: transport,
	}

	bot, err := tgbotapi.NewBotAPIWithClient(botToken, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil && update.CallbackQuery == nil {
			continue
		}

		var chatID int64
		var text string

		if update.Message != nil {
			chatID = update.Message.Chat.ID
			text = update.Message.Text
		} else if update.CallbackQuery != nil {
			chatID = update.CallbackQuery.Message.Chat.ID
			text = update.CallbackQuery.Data
		}

		session := getSession(chatID)

		// 1. Старт
		if (update.Message != nil && update.Message.IsCommand()) || text == "🔮 Новый расклад" {
			clearSession(chatID)
			setSession(chatID, &UserSession{Step: "awaiting_question"})
			
			msg := tgbotapi.NewMessage(chatID, "🔮 Добро пожаловать в Arcana AI!\n\nЗадайте ваш вопрос картам Таро (например: 'Что меня ждет в новом проекте?'):")
			msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
			bot.Send(msg)
			continue
		}

		// 2. Вопрос
		if session != nil && session.Step == "awaiting_question" && text != "" {
			log.Printf("DEBUG TG: User %d in step awaiting_question, received text: '%s'", chatID, text)
			session.Question = text
			session.Step = "awaiting_spread"
			setSession(chatID, session)

			msg := tgbotapi.NewMessage(chatID, "Отличный вопрос! Выберите тип расклада:")
			var buttons []tgbotapi.InlineKeyboardButton
			for _, sp := range availableSpreads {
				buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf("%s — %d₽", sp.Name, sp.Price),
					fmt.Sprintf("select_spread|%s", sp.ID),
				))
			}
			keyboard := tgbotapi.NewInlineKeyboardMarkup()
			for i := 0; i < len(buttons); i += 2 {
				if i+1 < len(buttons) {
					keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(buttons[i], buttons[i+1]))
				} else {
					keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(buttons[i]))
				}
			}
			msg.ReplyMarkup = keyboard
			
			if _, err := bot.Send(msg); err != nil {
				log.Printf("ERROR TG: Failed to send spread selection keyboard to chat %d: %v", chatID, err)
			} else {
				log.Printf("DEBUG TG: Keyboard sent successfully to chat %d", chatID)
			}
			continue
		}

		// 3. Выбор расклада и создание платежа через бэкенд
		if update.CallbackQuery != nil && strings.HasPrefix(text, "select_spread|") {
			parts := strings.Split(text, "|")
			if len(parts) == 2 {
				spreadID := parts[1]
				var selectedSpread *SpreadOption
				for i := range availableSpreads {
					if availableSpreads[i].ID == spreadID {
						selectedSpread = &availableSpreads[i]
						break
					}
				}

				if selectedSpread == nil {
					bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Ошибка"))
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка: расклад не найден."))
					continue
				}

				session.SelectedSpread = selectedSpread.Name
				session.SpreadPrice = selectedSpread.Price
				session.Step = "awaiting_payment"
				setSession(chatID, session)

				bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Расклад выбран"))
				bot.Send(tgbotapi.NewMessage(chatID, "⏳ Создаем ссылку на оплату..."))

				// Создаем платеж через бэкенд
				sessionID := generatePaymentToken()
				reqBody := map[string]interface{}{
					"session_id":  sessionID,
					"spread_name": selectedSpread.Name,
					"amount":      float64(selectedSpread.Price),
				}
				jsonData, _ := json.Marshal(reqBody)
				
				client := &http.Client{Timeout: 15 * time.Second}
				resp, err := client.Post(backendURL+"/api/create-payment", "application/json", bytes.NewBuffer(jsonData))
				
				if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
					log.Printf("ERROR TG: Failed to create payment for chat %d: err=%v, resp=%v", chatID, err, resp)
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось создать платеж. Попробуйте позже."))
					clearSession(chatID)
					continue
				}
				
				var payResp struct {
					PaymentID       string `json:"payment_id"`
					ConfirmationURL string `json:"confirmation_url"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&payResp); err != nil {
					log.Printf("ERROR TG: Failed to decode payment response for chat %d: %v", chatID, err)
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка при обработке ответа от платежной системы."))
					clearSession(chatID)
					continue
				}
				
				session.PaymentID = payResp.PaymentID
				session.PaymentCreatedAt = time.Now()
				setSession(chatID, session)

				msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
					"Вы выбрали: *%s*\n💰 Стоимость: *%d₽*\n\nДля продолжения ритуала необходимо оплатить расклад.\n\n🔗 [Нажмите здесь для оплаты через ЮKassa](%s)\n\nПосле успешной оплаты нажмите кнопку ниже.",
					session.SelectedSpread, session.SpreadPrice, payResp.ConfirmationURL,
				))
				msg.ParseMode = "Markdown"
				msg.DisableWebPagePreview = true
				msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonURL("💳 Оплатить расклад", payResp.ConfirmationURL),
					),
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("✅ Я оплатил", "check_payment"),
					),
				)
				bot.Send(msg)
			}
			continue
		}

		// 4. Проверка оплаты через бэкенд
		if update.CallbackQuery != nil && text == "check_payment" {
			if session == nil || session.PaymentID == "" {
				bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Сессия не найдена"))
				bot.Send(tgbotapi.NewMessage(chatID, "⏳ Сессия не найдена. Начните заново с /start"))
				continue
			}
			if time.Since(session.PaymentCreatedAt) > PaymentTokenLifetime {
				bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Токен истёк"))
				bot.Send(tgbotapi.NewMessage(chatID, "⏳ Время на оплату истекло. Начните заново с /start"))
				clearSession(chatID)
				continue
			}
			if session.PaymentPollingActive {
				bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Оплата уже проверяется"))
				continue
			}

			session.PaymentPollingActive = true
			setSession(chatID, session)

			bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Проверяем оплату..."))
			bot.Send(tgbotapi.NewMessage(chatID, "⏳ Проверяем статус оплаты в системе..."))
			
			go checkPaymentStatus(bot, chatID, session.PaymentID)
			continue
		}

		// 5. Срез колоды и ПОЭТАПНЫЙ РАСКЛАД
		if update.CallbackQuery != nil && text == "cut_deck" {
			bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "Колода срезана!"))

			selectedSpread := session.SelectedSpread
			question := session.Question
			deck := session.Deck

			cutIndex := mrand.Intn(len(deck))
			deck = append(deck[cutIndex:], deck[:cutIndex]...)

			bot.Send(tgbotapi.NewMessage(chatID, "🔮 Выкладываем карты на стол..."))

			cardCount := getCardCount(selectedSpread)
			var selectedCards []CardRequest
			var cardListText strings.Builder

			for i := 0; i < cardCount && i < len(deck); i++ {
				card := deck[i]
				selectedCards = append(selectedCards, CardRequest{
					Name:       card.Name,
					IsReversed: card.IsReversed,
				})
				
				reversedIcon := ""
				if card.IsReversed {
					reversedIcon = " 🔄"
				}
				cardListText.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, card.Name, reversedIcon))
			}

			cardsMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🃏 *Выпавшие карты:*\n\n%s", cardListText.String()))
			cardsMsg.ParseMode = "Markdown"
			bot.Send(cardsMsg)

			bot.Send(tgbotapi.NewMessage(chatID, "⏳ Запрашиваем мудрость ИИ для интерпретации..."))

			reqBody := ReadSpreadRequest{
				Question: question,
				Spread:   selectedSpread,
				Cards:    selectedCards,
			}
			jsonData, _ := json.Marshal(reqBody)
			client := &http.Client{Timeout: 60 * time.Second}
			resp, err := client.Post(backendURL+"/api/read-spread", "application/json", bytes.NewBuffer(jsonData))
			
			clearSession(chatID)

			if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
				log.Printf("ERROR TG: Backend request failed for chat %d: err=%v, resp=%v", chatID, err, resp)
				bot.Send(tgbotapi.NewMessage(chatID, "❌ Произошла ошибка при обращении к оракулу. Попробуйте позже."))
				continue
			}

			var result ReadSpreadResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				log.Printf("ERROR TG: Failed to decode backend response for chat %d: %v", chatID, err)
				bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка при обработке ответа от ИИ."))
				continue
			}

			log.Printf("DEBUG TG: Backend response for chat %d: intro=%d chars, cards=%d, conclusion=%d chars", 
				chatID, len(result.Intro), len(result.Cards), len(result.Conclusion))

			positions := getSpreadPositions(selectedSpread, len(result.Cards))

			// 1. Отправляем вступление
			if strings.TrimSpace(result.Intro) != "" {
				introMsg := tgbotapi.NewMessage(chatID, result.Intro)
				introMsg.ParseMode = "Markdown"
				if _, err := bot.Send(introMsg); err != nil {
					log.Printf("ERROR TG: Failed to send intro to chat %d: %v", chatID, err)
				}
				time.Sleep(1 * time.Second)
			}

			// 2. Отправляем карты по одной с паузой
			for i, cardInterp := range result.Cards {
				cardName := cardInterp.Card.Name
				isReversed := cardInterp.Card.IsReversed
				
				posText := ""
				if len(result.Cards) > 1 {
					posText = fmt.Sprintf("📍 *Позиция %d:* %s\n\n", i+1, positions[i])
				}
				
				reversedNote := ""
				if isReversed {
					reversedNote = " 🔄 *(Перевёрнутая)*"
				}
				
				caption := fmt.Sprintf("%s**%s**%s\n\n%s", posText, cardName, reversedNote, cardInterp.Interpretation)
				
				log.Printf("DEBUG TG: Sending card %d/%d (%s) to chat %d", i+1, len(result.Cards), cardName, chatID)
				
				imgURL := getCardImageURL(cardName)
				imgResp, err := http.Get(imgURL)
				if err != nil || imgResp.StatusCode != http.StatusOK {
					log.Printf("ERROR TG: Failed to download image for %s: %v. Sending text only...", cardName, err)
					textMsg := tgbotapi.NewMessage(chatID, caption)
					textMsg.ParseMode = "Markdown"
					bot.Send(textMsg)
				} else {
					imgBytes, _ := io.ReadAll(imgResp.Body)
					imgResp.Body.Close()
					
					photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{Name: "card.jpg", Bytes: imgBytes})
					photo.Caption = caption
					photo.ParseMode = "Markdown"
					
					if _, err := bot.Send(photo); err != nil {
						log.Printf("ERROR TG: Failed to send photo bytes for card %d: %v. Sending text only...", i+1, err)
						textMsg := tgbotapi.NewMessage(chatID, caption)
						textMsg.ParseMode = "Markdown"
						bot.Send(textMsg)
					}
				}
				
				if i < len(result.Cards)-1 {
					time.Sleep(1500 * time.Millisecond)
				}
			}

			// 3. Отправляем итог
			conclusion := strings.TrimSpace(result.Conclusion)
			if conclusion != "" {
				conclMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✨ *Итог*\n\n%s\n\n_Хотите задать еще один вопрос? Нажмите /start_", conclusion))
				conclMsg.ParseMode = "Markdown"
				if _, err := bot.Send(conclMsg); err != nil {
					log.Printf("ERROR TG: Failed to send conclusion to chat %d: %v", chatID, err)
				}
			} else {
				bot.Send(tgbotapi.NewMessage(chatID, "_Хотите задать еще один вопрос? Нажмите /start_"))
			}
			
			log.Printf("DEBUG TG: All cards sent to chat %d", chatID)
			continue
		}

		// Fallback
		if session == nil && update.Message != nil {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, используйте команду /start или кнопку '🔮 Новый расклад' для начала.")
			msg.ReplyMarkup = tgbotapi.NewReplyKeyboard(
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("🔮 Новый расклад"),
				),
			)
			bot.Send(msg)
		}
	}
}

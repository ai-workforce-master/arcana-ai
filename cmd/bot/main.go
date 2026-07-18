package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	mrand "math/rand"
	"net/http"
	"net/url"
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
	sessions      = make(map[int64]*UserSession)
	sessionsMutex sync.RWMutex
	botToken      = getEnv("TELEGRAM_BOT_TOKEN", "")
	// Используем 127.0.0.1, так как бот работает в network_mode: "host"
	backendURL = getEnv("BACKEND_URL", "http://127.0.0.1:18080")

	paymentSimDelay = make(map[int64]time.Duration)
	paymentSimMutex sync.RWMutex

	baseImageURL = "https://placehold.co/300x500/1a1a2e/FFF?text="
)

const (
	PaymentTokenLifetime = 1 * time.Hour
	PaymentCheckTimeout  = 10 * time.Minute
	PaymentPollInterval  = 5 * time.Second
)

// Полная колода из 78 карт Таро
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

// Список раскладов на основе документа "Расклады_Таро.docx"
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
			IsReversed: mrand.Float32() < 0.3, // 30% шанс перевернутой карты
		}
	}
	// Перемешиваем колоду (алгоритм Фишера-Йетса)
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

func simulatePaymentDelay(chatID int64) {
	delay := time.Duration(3+mrand.Intn(3)) * time.Second
	paymentSimMutex.Lock()
	paymentSimDelay[chatID] = delay
	paymentSimMutex.Unlock()
	log.Printf("DEBUG: Simulated payment delay for chat %d set to %v", chatID, delay)
}

func checkPayment(chatID int64, clickTime time.Time) bool {
	paymentSimMutex.RLock()
	delay, exists := paymentSimDelay[chatID]
	paymentSimMutex.RUnlock()
	if !exists {
		delay = 3 * time.Second
	}
	isPaid := time.Since(clickTime) >= delay
	if isPaid {
		log.Printf("DEBUG: Payment CONFIRMED for chat %d", chatID)
	}
	return isPaid
}

func getCardCount(spread string) int {
	s := strings.ToLower(spread)
	if strings.Contains(s, "одна") {
		return 1
	}
	if strings.Contains(s, "да или нет") {
		return 3
	}
	if strings.Contains(s, "три") {
		return 3
	}
	if strings.Contains(s, "крест") && !strings.Contains(s, "кельтский") {
		return 4
	}
	if strings.Contains(s, "слепое пятно") {
		return 4
	}
	if strings.Contains(s, "пятикарточн") {
		return 5
	}
	if strings.Contains(s, "ангел") || strings.Contains(s, "голос") {
		return 4
	}
	if strings.Contains(s, "пирамида") {
		return 6
	}
	if strings.Contains(s, "алхимик") {
		return 6
	}
	if strings.Contains(s, "подкова") {
		return 7
	}
	if strings.Contains(s, "звезда") || strings.Contains(s, "чакр") {
		return 7
	}
	if strings.Contains(s, "партнерство") || strings.Contains(s, "партнёрство") {
		return 7
	}
	if strings.Contains(s, "кельтский") {
		return 10
	}
	if strings.Contains(s, "колесо") {
		return 12
	}
	return 3 // Значение по умолчанию
}

func getCardImageURL(cardName string) string {
	encodedName := url.QueryEscape(cardName)
	urlStr := fmt.Sprintf("%s%s", baseImageURL, encodedName)
	log.Printf("DEBUG: Generated image URL for '%s': %s", cardName, urlStr)
	return urlStr
}

// --- Асинхронный поллер оплаты ---

func runPaymentPoller(bot *tgbotapi.BotAPI, chatID int64, token string, clickTime time.Time) {
	ticker := time.NewTicker(PaymentPollInterval)
	defer ticker.Stop()
	timeout := time.After(PaymentCheckTimeout)

	for {
		select {
		case <-ticker.C:
			session := getSession(chatID)
			if session == nil || session.PaymentToken != token {
				log.Printf("DEBUG: Poller stopped for chat %d (session nil or token mismatch)", chatID)
				return
			}
			if checkPayment(chatID, clickTime) {
				bot.Send(tgbotapi.NewMessage(chatID, "✅ Оплата подтверждена! Начинаем ритуал..."))
				go startShuffleAndReading(bot, chatID)
				return
			}
		case <-timeout:
			session := getSession(chatID)
			if session != nil && session.PaymentToken == token {
				remaining := PaymentTokenLifetime - time.Since(session.PaymentCreatedAt)
				var msg string
				if remaining > 0 {
					msg = fmt.Sprintf("⏳ К сожалению, оплата не была подтверждена в течение 10 минут.\n\n💡 Ваш токен ещё действителен (осталось %d минут). Вы можете попробовать снова, нажав '✅ Я оплатил', или начать заново с /start", int(remaining.Minutes()))
				} else {
					msg = "⏳ К сожалению, оплата не была подтверждена, и токен истёк. Начните заново с /start"
					clearSession(chatID)
				}
				bot.Send(tgbotapi.NewMessage(chatID, msg))
				session.PaymentPollingActive = false
				session.PaymentCheckUntil = time.Time{}
				setSession(chatID, session)
			}
			return
		}
	}
}

// --- Анимация тасовки и визуальный расклад ---

func startShuffleAndReading(bot *tgbotapi.BotAPI, chatID int64) {
	log.Printf("DEBUG: startShuffleAndReading called for chat %d", chatID)
	session := getSession(chatID)
	if session == nil {
		log.Printf("DEBUG: session is nil in startShuffleAndReading")
		return
	}

	session.Deck = generateDeck()
	session.Step = "shuffling"
	setSession(chatID, session)

	animMsg := tgbotapi.NewMessage(chatID, "🌪️ *Перемешиваем колоду...*")
	animMsg.ParseMode = "Markdown"
	sentMsg, _ := bot.Send(animMsg)
	log.Printf("DEBUG: Sent initial shuffle message, ID: %d", sentMsg.MessageID)

	time.Sleep(1200 * time.Millisecond)
	bot.Request(tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "🃏 *Карты выравниваются и заряжаются вашим намерением...*"))

	time.Sleep(1200 * time.Millisecond)
	bot.Request(tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "✨ *Колода готова к раскрытию тайн.*"))

	time.Sleep(800 * time.Millisecond)

	msg := tgbotapi.NewMessage(chatID, "Чтобы сделать расклад максимально личным, предложите колоде выбрать момент.\n\nНажмите кнопку ниже, чтобы **срезать колоду** (это случайным образом разделит её и переместит верхнюю часть вниз).")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✂️ Срезать колоду", "cut_deck"),
		),
	)
	bot.Send(msg)
	log.Printf("DEBUG: Shuffle animation complete, waiting for cut_deck")
}

// --- Основная логика бота ---

func main() {
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is not set")
	}

	// Создаем HTTP-клиент с таймаутом 120 секунд.
	// Это должно быть БОЛЬШЕ, чем u.Timeout = 60, чтобы не обрывать long-polling Telegram.
	httpClient := &http.Client{
		Timeout: 120 * time.Second,
	}

	// Инициализируем бота с кастомным HTTP-клиентом
	bot, err := tgbotapi.NewBotAPIWithClient(botToken, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	bot.Debug = false
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
			newSession := &UserSession{Step: "awaiting_question"}
			setSession(chatID, newSession)
			log.Printf("DEBUG: Started new session for chat %d", chatID)

			msg := tgbotapi.NewMessage(chatID, "🔮 Добро пожаловать в Arcana AI!\n\nЗадайте ваш вопрос картам Таро (например: 'Что меня ждет в новом проекте?'):")
			msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
			bot.Send(msg)
			continue
		}

		// 2. Вопрос
		if session != nil && session.Step == "awaiting_question" {
			if text == "" {
				continue
			}
			session.Question = text
			session.Step = "awaiting_spread"
			setSession(chatID, session)
			log.Printf("DEBUG: Question saved for chat %d: %s", chatID, text)

			msg := tgbotapi.NewMessage(chatID, "Отличный вопрос! Выберите тип расклада:")
			var buttons []tgbotapi.InlineKeyboardButton
			for _, sp := range availableSpreads {
				buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf("%s — %d₽", sp.Name, sp.Price),
					fmt.Sprintf("select_spread|%s|%d", sp.Name, sp.Price),
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
			bot.Send(msg)
			continue
		}

		// 3. Выбор расклада
		if update.CallbackQuery != nil && strings.HasPrefix(text, "select_spread|") {
			parts := strings.Split(text, "|")
			if len(parts) == 3 {
				session.SelectedSpread = parts[1]
				fmt.Sscanf(parts[2], "%d", &session.SpreadPrice)
				session.PaymentToken = generatePaymentToken()
				session.PaymentCreatedAt = time.Now()
				session.Step = "awaiting_payment"
				setSession(chatID, session)
				simulatePaymentDelay(chatID)

				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Расклад выбран")
				bot.Request(callback)

				msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
					"Вы выбрали: *%s*\n💰 Стоимость: *%d₽*\n\n📋 *Номер платежа:* `%s`\n⏰ Токен действителен 1 час\n\nПереведите сумму на карту: `2200 0000 0000 0000` (Тинькофф)\n\nПосле перевода нажмите кнопку ниже.",
					session.SelectedSpread, session.SpreadPrice, session.PaymentToken,
				))
				msg.ParseMode = "Markdown"
				msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("✅ Я оплатил", "check_payment"),
					),
				)
				bot.Send(msg)
				log.Printf("DEBUG: Payment prompt sent for chat %d", chatID)
			}
			continue
		}

		// 4. Проверка оплаты
		if update.CallbackQuery != nil && text == "check_payment" {
			if time.Since(session.PaymentCreatedAt) > PaymentTokenLifetime {
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Токен истёк")
				bot.Request(callback)
				bot.Send(tgbotapi.NewMessage(chatID, "⏳ Токен платежа истёк (прошёл более 1 часа). Начните заново с /start"))
				clearSession(chatID)
				continue
			}
			if session.PaymentPollingActive {
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Оплата уже проверяется")
				bot.Request(callback)
				continue
			}

			session.PaymentClickTime = time.Now()
			session.PaymentCheckUntil = time.Now().Add(PaymentCheckTimeout)
			session.PaymentPollingActive = true
			setSession(chatID, session)

			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Проверяем оплату...")
			bot.Request(callback)
			bot.Send(tgbotapi.NewMessage(chatID, "⏳ Проверяем оплату в системе... Это займет несколько секунд.\n\n💡 Не закрывайте чат, мы сообщим о результате."))
			log.Printf("DEBUG: Payment poller started for chat %d", chatID)
			go runPaymentPoller(bot, chatID, session.PaymentToken, session.PaymentClickTime)
			continue
		}

		// 5. Срез колоды и ВИЗУАЛЬНЫЙ РАСКЛАД
		if update.CallbackQuery != nil && text == "cut_deck" {
			log.Printf("DEBUG: cut_deck callback received for chat %d", chatID)
			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Колода срезана!")
			bot.Request(callback)

			selectedSpread := session.SelectedSpread
			question := session.Question
			deck := session.Deck

			cutIndex := mrand.Intn(len(deck))
			deck = append(deck[cutIndex:], deck[:cutIndex]...)
			log.Printf("DEBUG: Deck cut at index %d", cutIndex)

			bot.Send(tgbotapi.NewMessage(chatID, "🔮 Выкладываем карты на стол..."))

			cardCount := getCardCount(selectedSpread)
			var selectedCards []CardRequest
			var mediaGroup []interface{}

			for i := 0; i < cardCount && i < len(deck); i++ {
				card := deck[i]
				selectedCards = append(selectedCards, CardRequest{
					Name:       card.Name,
					IsReversed: card.IsReversed,
				})

				photo := tgbotapi.NewInputMediaPhoto(tgbotapi.FileURL(getCardImageURL(card.Name)))

				// КРИТИЧЕСКИ ВАЖНО: Telegram разрешает подпись (Caption) ТОЛЬКО для первого элемента в MediaGroup!
				if i == 0 {
					reversedIcon := ""
					if card.IsReversed {
						reversedIcon = " 🔄 (Перевёрнутая)"
					}
					photo.Caption = fmt.Sprintf("🔮 *Расклад: %s*\n\nПозиция %d: *%s*%s", selectedSpread, i+1, card.Name, reversedIcon)
					photo.ParseMode = "Markdown"
				}

				mediaGroup = append(mediaGroup, photo)
			}

			if len(mediaGroup) > 0 {
				log.Printf("DEBUG: Attempting to send MediaGroup with %d items", len(mediaGroup))
				_, err := bot.Send(tgbotapi.NewMediaGroup(chatID, mediaGroup))
				if err != nil {
					log.Printf("ERROR: Failed to send MediaGroup: %v", err)
				} else {
					log.Printf("DEBUG: MediaGroup sent successfully")
				}
			}

			bot.Send(tgbotapi.NewMessage(chatID, "⏳ Запрашиваем мудрость ИИ для интерпретации этого расклада..."))

			reqBody := ReadSpreadRequest{
				Question: question,
				Spread:   selectedSpread,
				Cards:    selectedCards,
			}
			jsonData, _ := json.Marshal(reqBody)
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Post(backendURL+"/api/read-spread", "application/json", bytes.NewBuffer(jsonData))

			clearSession(chatID)

			if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
				errMsg := "❌ Произошла ошибка при обращении к оракулу. Попробуйте позже."
				if err != nil {
					errMsg += fmt.Sprintf("\n(Детали: %v)", err)
				}
				bot.Send(tgbotapi.NewMessage(chatID, errMsg))
				continue
			}

			var result ReadSpreadResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка при обработке ответа от ИИ."))
				continue
			}

			answerText := fmt.Sprintf("🔮 **Расклад: %s**\n📜 **Вопрос:** _%s_\n\n%s\n\n_Хотите задать еще один вопрос? Нажмите /start_",
				selectedSpread, question, result.Interpretation)

			answerMsg := tgbotapi.NewMessage(chatID, answerText)
			answerMsg.ParseMode = "Markdown"
			bot.Send(answerMsg)
			log.Printf("DEBUG: Final interpretation sent for chat %d", chatID)
			continue
		}

		// Fallback
		if session == nil {
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

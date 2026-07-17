package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"net/url"

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
	backendURL    = getEnv("BACKEND_URL", "http://backend:8080")

	paymentSimDelay = make(map[int64]time.Duration)
	paymentSimMutex sync.RWMutex

	// Базовый URL для изображений карт. 
	// ЗАМЕНИТЕ это на ваш хостинг с реальными картинками (например, https://yourdomain.com/cards/)
	baseImageURL = "https://placehold.co/300x500/1a1a2e/FFF?text="
)

// Соответствие русских названий файлов для картинок (для будущего использования с реальными изображениями)
var cardImageFiles = map[string]string{
	"Шут": "fool", "Маг": "magician", "Верховная Жрица": "high_priestess", "Императрица": "empress",
	"Император": "emperor", "Иерофант": "hierophant", "Влюбленные": "lovers", "Колесница": "chariot",
	"Сила": "strength", "Отшельник": "hermit", "Колесо Фортуны": "wheel_of_fortune", "Справедливость": "justice",
	"Повешенный": "hanged_man", "Смерть": "death", "Умеренность": "temperance", "Дьявол": "devil",
	"Башня": "tower", "Звезда": "star", "Луна": "moon", "Солнце": "sun", "Суд": "judgement", "Мир": "world",
	// Для младших арканов можно добавить аналогичные маппинги или использовать генерацию имени файла
}

const (
	PaymentTokenLifetime = 1 * time.Hour
	PaymentCheckTimeout  = 10 * time.Minute
	PaymentPollInterval  = 10 * time.Second
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
	{"Пятикарточный расклад", 250},
	{"Кельтский крест (10 карт)", 500},
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

func simulatePaymentDelay(chatID int64) {
	delay := time.Duration(5+mrand.Intn(25)) * time.Second
	paymentSimMutex.Lock()
	paymentSimDelay[chatID] = delay
	paymentSimMutex.Unlock()
}

func checkPayment(chatID int64, clickTime time.Time) bool {
	paymentSimMutex.RLock()
	delay, exists := paymentSimDelay[chatID]
	paymentSimMutex.RUnlock()
	if !exists {
		delay = 5 * time.Second
	}
	return time.Since(clickTime) >= delay
}

func getCardCount(spread string) int {
	s := strings.ToLower(spread)
	if strings.Contains(s, "одна") { return 1 }
	if strings.Contains(s, "да или нет") { return 3 }
	if strings.Contains(s, "три") { return 3 }
	if strings.Contains(s, "крест") && !strings.Contains(s, "кельтский") { return 4 }
	if strings.Contains(s, "пятикарточн") { return 5 }
	if strings.Contains(s, "кельтский") { return 10 }
	return 3
}

func getCardImageURL(cardName string) string {
	// Если у вас есть реальные картинки, раскомментируйте и адаптируйте эту логику:
	// fileName, exists := cardImageFiles[cardName]
	// if !exists { fileName = strings.ReplaceAll(cardName, " ", "_") }
	// return fmt.Sprintf("%s%s.jpg", baseImageURL, fileName)
	
	// Пока используем надежную заглушку, которая генерирует картинку с названием карты
	encodedName := url.QueryEscape(cardName)
	return fmt.Sprintf("%s%s", baseImageURL, encodedName)
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
	session := getSession(chatID)
	if session == nil { return }

	session.Deck = generateDeck()
	session.Step = "shuffling"
	setSession(chatID, session)

	// 1. Анимация тасовки через быструю правку одного сообщения
	animMsg := tgbotapi.NewMessage(chatID, "🌪️ *Перемешиваем колоду...*")
	animMsg.ParseMode = "Markdown"
	sentMsg, _ := bot.Send(animMsg)

	time.Sleep(1200 * time.Millisecond)
	bot.Send(tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "🃏 *Карты выравниваются и заряжаются вашим намерением...*"))
	
	time.Sleep(1200 * time.Millisecond)
	bot.Send(tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "✨ *Колода готова к раскрытию тайн.*"))
	
	time.Sleep(800 * time.Millisecond)

	msg := tgbotapi.NewMessage(chatID, "Чтобы сделать расклад максимально личным, предложите колоде выбрать момент.\n\nНажмите кнопку ниже, чтобы **срезать колоду** (это случайным образом разделит её и переместит верхнюю часть вниз).")
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

	bot, err := tgbotapi.NewBotAPI(botToken)
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

			msg := tgbotapi.NewMessage(chatID, "🔮 Добро пожаловать в Arcana AI!\n\nЗадайте ваш вопрос картам Таро (например: 'Что меня ждет в новом проекте?'):")
			msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
			bot.Send(msg)
			continue
		}

		// 2. Вопрос
		if session != nil && session.Step == "awaiting_question" {
			if text == "" { continue }
			session.Question = text
			session.Step = "awaiting_spread"
			setSession(chatID, session)

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
			bot.Send(tgbotapi.NewMessage(chatID, "⏳ Проверяем оплату в системе... Это может занять до 10 минут.\n\n💡 Не закрывайте чат, мы сообщим о результате."))
			go runPaymentPoller(bot, chatID, session.PaymentToken, session.PaymentClickTime)
			continue
		}

		// 5. Срез колоды и ВИЗУАЛЬНЫЙ РАСКЛАД
		if update.CallbackQuery != nil && text == "cut_deck" {
			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Колода срезана!")
			bot.Request(callback)

			selectedSpread := session.SelectedSpread
			question := session.Question
			deck := session.Deck

			cutIndex := mrand.Intn(len(deck))
			deck = append(deck[cutIndex:], deck[:cutIndex]...)

			bot.Send(tgbotapi.NewMessage(chatID, "🔮 Выкладываем карты на стол..."))

			cardCount := getCardCount(selectedSpread)
			var selectedCards []CardRequest
			
			// Формируем медиа-группу для визуального отображения карт (максимум 10 в Telegram)
			var mediaGroup []interface{}
			
			for i := 0; i < cardCount && i < len(deck); i++ {
				card := deck[i]
				selectedCards = append(selectedCards, CardRequest{
					Name:       card.Name,
					IsReversed: card.IsReversed,
				})

				// Создаем элемент медиа-группы
				reversedIcon := ""
				if card.IsReversed {
					reversedIcon = " 🔄 (Перевёрнутая)"
				}
				
				photo := tgbotapi.NewInputMediaPhoto(tgbotapi.FileURL(getCardImageURL(card.Name)))
				// Caption поддерживает Markdown, но в MediaGroup он ограничен. Используем простой текст.
				photo.Caption = fmt.Sprintf("Позиция %d\n%s%s", i+1, card.Name, reversedIcon)
				photo.ParseMode = "Markdown"
				
				mediaGroup = append(mediaGroup, photo)
			}

			// Отправляем карты альбомом (это выглядит как настоящий расклад на столе)
			if len(mediaGroup) > 0 {
				bot.Send(tgbotapi.NewMediaGroup(chatID, mediaGroup))
			}

			bot.Send(tgbotapi.NewMessage(chatID, "⏳ Запрашиваем мудрость ИИ для интерпретации этого расклада..."))

			// Вызов бэкенда
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

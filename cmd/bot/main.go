package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
	Step        string
	Question    string
	Spread      string
	Cards       []CardRequest
	TotalCards  int
	CurrentCard int
	CreatedAt   time.Time
}

var (
	sessions      = make(map[int64]*UserSession)
	sessionsMutex sync.RWMutex
	botToken      = getEnv("TELEGRAM_BOT_TOKEN", "")
	backendURL    = getEnv("BACKEND_URL", "http://backend:8080")
)

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getCardCount(spread string) int {
	s := strings.ToLower(spread)
	if strings.Contains(s, "одна карт") || strings.Contains(s, "карта дня") {
		return 1
	}
	if strings.Contains(s, "да или нет") || strings.Contains(s, "да/нет") {
		return 3
	}
	if strings.Contains(s, "три карт") || strings.Contains(s, "триплет") {
		return 3
	}
	if strings.Contains(s, "крест") && !strings.Contains(s, "кельтский") {
		return 4
	}
	if strings.Contains(s, "пятикарточн") || strings.Contains(s, "пять карт") {
		return 5
	}
	if strings.Contains(s, "пирамида") || strings.Contains(s, "алхимик") {
		return 6
	}
	if strings.Contains(s, "подкова") || strings.Contains(s, "звезда") || strings.Contains(s, "чакр") || strings.Contains(s, "партнерство") || strings.Contains(s, "партнёрство") {
		return 7
	}
	if strings.Contains(s, "кельтский") {
		return 10
	}
	if strings.Contains(s, "колесо") {
		return 12
	}
	if strings.Contains(s, "слепое пятно") || strings.Contains(s, "ангел") || strings.Contains(s, "голос") {
		return 4
	}
	return 3 // По умолчанию
}

func parseCardInput(text string) (name string, isReversed bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	isReversed = strings.Contains(lower, "пер") || strings.Contains(lower, "перевернут") || strings.Contains(lower, "rev")

	name = strings.ReplaceAll(lower, "перевернутая", "")
	name = strings.ReplaceAll(name, "перевернутый", "")
	name = strings.ReplaceAll(name, "пер", "")
	name = strings.ReplaceAll(name, "rev", "")
	name = strings.ReplaceAll(name, "reversed", "")
	name = strings.TrimSpace(name)

	if len(name) > 0 {
		runes := []rune(name)
		runes[0] = unicode.ToUpper(runes[0])
		name = string(runes)
	}
	return name, isReversed
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
	session.CreatedAt = time.Now()
	sessions[chatID] = session
	sessionsMutex.Unlock()
}

func cleanupSessions() {
	for {
		time.Sleep(5 * time.Minute)
		sessionsMutex.Lock()
		now := time.Now()
		for id, sess := range sessions {
			if now.Sub(sess.CreatedAt) > 15*time.Minute {
				delete(sessions, id)
			}
		}
		sessionsMutex.Unlock()
	}
}

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

	go cleanupSessions()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		chatID := update.Message.Chat.ID
		text := update.Message.Text

		// Обработка команд
		if update.Message.IsCommand() {
			clearSession(chatID)
			if update.Message.Command() == "start" {
				msg := tgbotapi.NewMessage(chatID, "🔮 Добро пожаловать в Arcana AI!\n\nЯ помогу вам интерпретировать расклад Таро с помощью искусственного интеллекта.\n\nНажмите кнопку ниже, чтобы начать новый расклад.")
				keyboard := tgbotapi.NewReplyKeyboard(
					tgbotapi.NewKeyboardButtonRow(
						tgbotapi.NewKeyboardButton("🔮 Новый расклад"),
					),
				)
				msg.ReplyMarkup = keyboard
				bot.Send(msg)
			} else if update.Message.Command() == "help" {
				msg := tgbotapi.NewMessage(chatID, "📜 Инструкция:\n1. Нажмите '🔮 Новый расклад'\n2. Введите ваш вопрос\n3. Выберите тип расклада из меню\n4. По очереди вводите названия выпавших карт.\n💡 Совет: чтобы указать перевернутую карту, добавьте 'пер' (например: 'Шут пер' или 'Маг перевернутая').")
				bot.Send(msg)
			}
			continue
		}

		// Обработка текстовых сообщений и кнопок
		session := getSession(chatID)

		if text == "🔮 Новый расклад" || (session != nil && session.Step == "question") {
			clearSession(chatID)
			newSession := &UserSession{Step: "question"}
			setSession(chatID, newSession)
			msg := tgbotapi.NewMessage(chatID, "Задайте ваш вопрос картам Таро (например: 'Что меня ждет в новом проекте?'):")
			msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
			bot.Send(msg)
			continue
		}

		if session != nil && session.Step == "question" {
			session.Question = text
			session.Step = "spread"
			setSession(chatID, session)

			msg := tgbotapi.NewMessage(chatID, "Отличный вопрос! Выберите тип расклада:")
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Одна карта", "spread_Одна карта"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Да или Нет", "spread_Да или Нет"),
					tgbotapi.NewInlineKeyboardButtonData("Три карты", "spread_Три карты"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Крест", "spread_Крест"),
					tgbotapi.NewInlineKeyboardButtonData("Пятикарточный", "spread_Пятикарточный"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Кельтский крест", "spread_Кельтский крест"),
				),
			)
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
			continue
		}

		if update.CallbackQuery != nil {
			if strings.HasPrefix(update.CallbackQuery.Data, "spread_") {
				spreadName := strings.TrimPrefix(update.CallbackQuery.Data, "spread_")
				session.Spread = spreadName
				session.TotalCards = getCardCount(spreadName)
				session.CurrentCard = 1
				session.Cards = []CardRequest{}
				session.Step = "card"
				setSession(chatID, session)

				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Выбрано: "+spreadName)
				bot.Request(callback)

				msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Расклад '%s' выбран. Всего карт: %d.\n\nВведите название **первой** карты (например: 'Шут' или 'Шут пер' для перевернутой):", spreadName, session.TotalCards))
				msg.ParseMode = "Markdown"
				bot.Send(msg)
			}
			continue
		}

		if session != nil && session.Step == "card" {
			name, isReversed := parseCardInput(text)
			session.Cards = append(session.Cards, CardRequest{Name: name, IsReversed: isReversed})
			session.CurrentCard++

			if session.CurrentCard > session.TotalCards {
				session.Step = "processing"
				setSession(chatID, session)

				msg := tgbotapi.NewMessage(chatID, "⏳ Анализирую расклад с помощью ИИ... Это займет несколько секунд.")
				bot.Send(msg)

				// Вызов бэкенда
				reqBody := ReadSpreadRequest{
					Question: session.Question,
					Spread:   session.Spread,
					Cards:    session.Cards,
				}
				jsonData, _ := json.Marshal(reqBody)
				resp, err := http.Post(backendURL+"/api/read-spread", "application/json", bytes.NewBuffer(jsonData))
				
				clearSession(chatID) // Очищаем сессию сразу после запроса

				if err != nil || resp.StatusCode != http.StatusOK {
					errMsg := "❌ Произошла ошибка при обращении к серверу. Попробуйте позже."
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

				answerMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔮 **Интерпретация расклада:**\n\n%s\n\n_Хотите сделать еще один расклад? Нажмите '🔮 Новый расклад'._", result.Interpretation))
				answerMsg.ParseMode = "Markdown"
				
				keyboard := tgbotapi.NewReplyKeyboard(
					tgbotapi.NewKeyboardButtonRow(
						tgbotapi.NewKeyboardButton("🔮 Новый расклад"),
					),
				)
				answerMsg.ReplyMarkup = keyboard
				bot.Send(answerMsg)
			} else {
				setSession(chatID, session)
				msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Карта '%s' принята.\n\nВведите название **%d-й** карты (из %d):", name, session.CurrentCard, session.TotalCards))
				bot.Send(msg)
			}
			continue
		}

		// Если сообщение не распознано
		if session == nil {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, используйте команду /start или кнопку '🔮 Новый расклад' для начала.")
			keyboard := tgbotapi.NewReplyKeyboard(
				tgbotapi.NewKeyboardButtonRow(
					tgbotapi.NewKeyboardButton("🔮 Новый расклад"),
				),
			)
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
		}
	}
}

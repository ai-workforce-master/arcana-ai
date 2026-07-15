package ai

import (
	"context"
	"arcana-ai/internal/config"
	openai "github.com/sashabaranov/go-openai"
)

type Client struct {
	chatClient *openai.Client
	cfg        *config.Config
}

func NewClient(cfg *config.Config) *Client {
	clientConfig := openai.DefaultConfig(cfg.AITunnelAPIKey)
	clientConfig.BaseURL = cfg.AITunnelBaseURL
	
	return &Client{
		chatClient: openai.NewClientWithConfig(clientConfig),
		cfg:        cfg,
	}
}

// ChatCompletion отправляет запрос к gpt-4o-mini
func (c *Client) ChatCompletion(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	resp, err := c.chatClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: messages,
	})
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

// Transcribe отправляет аудио на распознавание (gpt-4o-mini-transcribe)
func (c *Client) Transcribe(ctx context.Context, audioFilePath string) (string, error) {
	req := openai.AudioRequest{
		Model:    "gpt-4o-mini-transcribe",
		FilePath: audioFilePath,
		Language: "ru",
	}
	resp, err := c.chatClient.CreateTranscription(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// GenerateSpeech использует gpt-4o-mini-tts для синтеза речи
func (c *Client) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	req := openai.CreateSpeechRequest{
		Model: openai.TTSModel1, // Примечание: AITunnel должен мапить это на gpt-4o-mini-tts
		Input: text,
		Voice: openai.VoiceAlloy, // Можно параметризовать
	}
	resp, err := c.chatClient.CreateSpeech(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	
	// Чтение всего аудио в память (для MVP допустимо, для продакшена лучше стриминг или сохранение на диск)
	return resp.Read()
}

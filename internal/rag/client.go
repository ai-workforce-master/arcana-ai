package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"arcana-ai/internal/config"
	"github.com/qdrant/go-client/qdrant"
)

// CardData хранит данные одной карты для сборки промпта LLM
type CardData struct {
	Name     string
	Position string // Например: "1. Прошлое"
	Upright  string
	Reversed string
	Advice   string
}

type Client struct {
	embeddingsURL string
	qdrantClient  *qdrant.Client
	collection    string
	httpClient    *http.Client
}

func NewClient(cfg *config.Config) (*Client, error) {
	u, err := url.Parse(cfg.QdrantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Qdrant URL: %w", err)
	}

	portStr := u.Port()
	if portStr == "6333" {
		portStr = "6334"
	} else if portStr == "16333" {
		portStr = "16334"
	} else if portStr == "" {
		portStr = "6334"
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 6334
	}

	qdrantCfg := &qdrant.Config{
		Host:   u.Hostname(),
		Port:   port,
		UseTLS: u.Scheme == "https",
	}

	client, err := qdrant.NewClient(qdrantCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	return &Client{
		embeddingsURL: cfg.EmbeddingsSvcURL,
		qdrantClient:  client,
		collection:    "tarot_cards",
		httpClient:    &http.Client{},
	}, nil
}

type EmbedResponse struct {
	Embedding []float64 `json:"embedding"`
	Source    string    `json:"source"`
}

func (c *Client) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", c.embeddingsURL+"/embed", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var embedResp EmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w, body: %s", err, string(body))
	}

	vec32 := make([]float32, len(embedResp.Embedding))
	for i, v := range embedResp.Embedding {
		vec32[i] = float32(v)
	}
	return vec32, nil
}

type SearchResult struct {
	Name     string  `json:"name"`
	Arcana   string  `json:"arcana"`
	Keywords string  `json:"keywords"`
	Upright  string  `json:"upright"`
	Reversed string  `json:"reversed"`
	Advice   string  `json:"advice"`
	Score    float32 `json:"score"`
}

// Search — старый метод для тестового эндпоинта (семантический поиск)
func (c *Client) Search(ctx context.Context, query string, limit uint64) ([]SearchResult, error) {
	vec, err := c.GetEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	searchPoints := &qdrant.QueryPoints{
		CollectionName: c.collection,
		Query:          qdrant.NewQueryDense(vec),
		Limit:          &limit,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	}

	results, err := c.qdrantClient.Query(ctx, searchPoints)
	if err != nil {
		return nil, fmt.Errorf("qdrant query failed: %w", err)
	}

	var searchResults []SearchResult
	for _, hit := range results {
		payload := hit.GetPayload()
		res := SearchResult{Score: hit.GetScore()}

		if v, ok := payload["name"]; ok {
			res.Name = v.GetStringValue()
		}
		if v, ok := payload["arcana"]; ok {
			res.Arcana = v.GetStringValue()
		}
		if v, ok := payload["keywords"]; ok {
			res.Keywords = v.GetStringValue()
		}
		if v, ok := payload["upright"]; ok {
			res.Upright = v.GetStringValue()
		}
		if v, ok := payload["reversed"]; ok {
			res.Reversed = v.GetStringValue()
		}
		if v, ok := payload["advice"]; ok {
			res.Advice = v.GetStringValue()
		}
		searchResults = append(searchResults, res)
	}

	return searchResults, nil
}

// GetCardsData ищет данные для списка названий карт.
// Для каждого названия делаем отдельный точечный поиск (limit=1),
// так как семантическая близость имени карты к самому себе максимальна.
func (c *Client) GetCardsData(ctx context.Context, cardNames []string, spreadPositions []string) ([]CardData, error) {
	var results []CardData
	var limit uint64 = 1

	for i, name := range cardNames {
		// Определяем название позиции
		posName := fmt.Sprintf("Позиция %d", i+1)
		if i < len(spreadPositions) && spreadPositions[i] != "" {
			posName = fmt.Sprintf("%d. %s", i+1, spreadPositions[i])
		}

		// Получаем вектор ТОЛЬКО для названия карты (короткий запрос = точное попадание)
		vec, err := c.GetEmbedding(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("failed to embed card name '%s': %w", name, err)
		}

		searchPoints := &qdrant.QueryPoints{
			CollectionName: c.collection,
			Query:          qdrant.NewQueryDense(vec),
			Limit:          &limit,
			WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		}

		hits, err := c.qdrantClient.Query(ctx, searchPoints)
		if err != nil {
			return nil, fmt.Errorf("qdrant query failed for '%s': %w", name, err)
		}

		if len(hits) > 0 {
			hit := hits[0]
			payload := hit.GetPayload()

			card := CardData{
				Position: posName,
			}
			if v, ok := payload["name"]; ok {
				card.Name = v.GetStringValue()
			}
			if v, ok := payload["upright"]; ok {
				card.Upright = v.GetStringValue()
			}
			if v, ok := payload["reversed"]; ok {
				card.Reversed = v.GetStringValue()
			}
			if v, ok := payload["advice"]; ok {
				card.Advice = v.GetStringValue()
			}
			results = append(results, card)
		}
	}

	return results, nil
}

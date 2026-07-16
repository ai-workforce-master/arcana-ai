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
	"strings"

	"arcana-ai/internal/config"
	"github.com/qdrant/go-client/qdrant"
)

// CardData хранит данные одной карты для сборки промпта LLM
type CardData struct {
	Name     string
	Position string
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

// GetCardsData использует Scroll для 100% точного поиска по названию.
// Поскольку карт всего 78, загрузка всех записей в память занимает < 5 мс и гарантирует 
// отсутствие ошибок векторного поиска для коротких строк.
func (c *Client) GetCardsData(ctx context.Context, cardNames []string, spreadPositions []string) ([]CardData, error) {
	var limit uint32 = 100 // Больше, чем 78 карт в колоде

	scrollPoints := &qdrant.ScrollPoints{
		CollectionName: c.collection,
		Limit:          &limit,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	}

	result, err := c.qdrantClient.Scroll(ctx, scrollPoints)
	if err != nil {
		return nil, fmt.Errorf("qdrant scroll failed: %w", err)
	}

	// Строим карту для мгновенного O(1) поиска по названию (в нижнем регистре)
	cardMap := make(map[string]CardData)
	for _, point := range result.GetResult() {
		payload := point.GetPayload()
		var name, upright, reversed, advice string
		
		if v, ok := payload["name"]; ok {
			name = v.GetStringValue()
		}
		if v, ok := payload["upright"]; ok {
			upright = v.GetStringValue()
		}
		if v, ok := payload["reversed"]; ok {
			reversed = v.GetStringValue()
		}
		if v, ok := payload["advice"]; ok {
			advice = v.GetStringValue()
		}

		if name != "" {
			cardMap[strings.ToLower(strings.TrimSpace(name))] = CardData{
				Name:     name,
				Upright:  upright,
				Reversed: reversed,
				Advice:   advice,
			}
		}
	}

	var results []CardData
	for i, reqName := range cardNames {
		posName := fmt.Sprintf("Позиция %d", i+1)
		if i < len(spreadPositions) && spreadPositions[i] != "" {
			posName = fmt.Sprintf("%d. %s", i+1, spreadPositions[i])
		}

		lookupName := strings.ToLower(strings.TrimSpace(reqName))
		if card, exists := cardMap[lookupName]; exists {
			card.Position = posName
			results = append(results, card)
		} else {
			// Fallback, если карта не найдена (например, опечатка в названии)
			results = append(results, CardData{
				Name:     reqName,
				Position: posName,
				Upright:  "Карта не найдена в базе знаний. Проверьте правильность названия.",
				Reversed: "",
				Advice:   "Убедитесь, что название карты совпадает с классическим названием Таро.",
			})
		}
	}

	return results, nil
}

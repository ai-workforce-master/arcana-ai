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
	qdrantclient "github.com/qdrant/go-client"
	"github.com/qdrant/go-client/qdrant"
)

type Client struct {
	embeddingsURL string
	qdrantClient  *qdrantclient.Client
	collection    string
	httpClient    *http.Client
}

func NewClient(cfg *config.Config) (*Client, error) {
	u, err := url.Parse(cfg.QdrantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Qdrant URL: %w", err)
	}

	// Go-клиент Qdrant использует gRPC (порт 6334), а не HTTP (порт 6333).
	portStr := u.Port()
	if portStr == "6333" {
		portStr = "6334"
	} else if portStr == "16333" {
		portStr = "16334"
	} else if portStr == "" {
		portStr = "6334"
	}

	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		port = 6334
	}

	qdrantCfg := &qdrantclient.Config{
		Host:   u.Hostname(),
		Port:   uint32(port),
		UseTLS: u.Scheme == "https",
	}

	client, err := qdrantclient.NewClient(qdrantCfg)
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

	searchPoints := &qdrant.SearchPoints{
		CollectionName: c.collection,
		Vector:         vec,
		Limit:          &limit,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
	}

	results, err := c.qdrantClient.Search(ctx, searchPoints)
	if err != nil {
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	var searchResults []SearchResult
	for _, hit := range results.GetResult() {
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

package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type YooKassaService struct {
	shopID        string
	secretKey     string
	returnURL     string
	failURL       string
	webhookSecret string
	baseURL       string
	httpClient    *http.Client
}

type PaymentRecord struct {
	ID         string
	SessionID  string
	YooKassaID string
	Amount     float64
	Status     string
	SpreadName string
	TaxAmount  float64
	TaxRate    float64
	CreatedAt  time.Time
	PaidAt     *time.Time
	ReceiptURL sql.NullString // ИСПРАВЛЕНО: корректная обработка NULL из БД
}

type WebhookPayload struct {
	Type   string          `json:"type"`
	Event  string          `json:"event"`
	Object json.RawMessage `json:"object"`
}

type PaymentObject struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Paid   bool   `json:"paid"`
}

func NewYooKassaService() (*YooKassaService, error) {
	shopID := os.Getenv("YOOKASSA_SHOP_ID")
	secretKey := os.Getenv("YOOKASSA_SECRET_KEY")
	returnURL := os.Getenv("YOOKASSA_RETURN_URL")
	failURL := os.Getenv("YOOKASSA_FAIL_URL")
	webhookSecret := os.Getenv("YOOKASSA_WEBHOOK_SECRET")

	if shopID == "" || secretKey == "" {
		return nil, fmt.Errorf("YOOKASSA_SHOP_ID and YOOKASSA_SECRET_KEY are required")
	}

	if returnURL == "" {
		returnURL = "https://ai-arcana.ru/payment-success"
	}
	if failURL == "" {
		failURL = "https://ai-arcana.ru/payment-fail"
	}

	return &YooKassaService{
		shopID:        shopID,
		secretKey:     secretKey,
		returnURL:     returnURL,
		failURL:       failURL,
		webhookSecret: webhookSecret,
		baseURL:       "https://api.yookassa.ru/v3",
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (s *YooKassaService) getAuthHeader() string {
	credentials := s.shopID + ":" + s.secretKey
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
}

func (s *YooKassaService) CreatePayment(ctx context.Context, db *sql.DB, sessionID, spreadName string, amount float64) (string, string, error) {
	idempotencyKey := uuid.New().String()
	log.Printf("DEBUG: Generated Idempotence-Key: [%s]", idempotencyKey)

	reqBody := map[string]interface{}{
		"amount": map[string]string{
			"value":    fmt.Sprintf("%.2f", amount),
			"currency": "RUB",
		},
		"confirmation": map[string]string{
			"type":       "redirect",
			"return_url": s.returnURL,
		},
		"capture":         true,
		"description":     fmt.Sprintf("Расклад Таро: %s", spreadName),
		"idempotency_key": idempotencyKey,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/payments", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotencyKey)
	req.Header.Set("Authorization", s.getAuthHeader())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to send request to YooKassa: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("YooKassa API error: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		Confirmation struct {
			Type            string `json:"type"`
			ConfirmationURL string `json:"confirmation_url"`
		} `json:"confirmation"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode YooKassa response: %w", err)
	}

	if result.Confirmation.ConfirmationURL == "" {
		return "", "", fmt.Errorf("no confirmation URL in YooKassa response")
	}

	taxAmount := amount * 0.04
	query := `
		INSERT INTO payments 
		(session_id, yookassa_id, amount, status, spread_name, tax_amount, tax_rate, idempotency_key)
		VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7)
		RETURNING id
	`

	var paymentID string
	err = db.QueryRowContext(ctx, query,
		sessionID, result.ID, amount, spreadName, taxAmount, 0.04, idempotencyKey,
	).Scan(&paymentID)

	if err != nil {
		return "", "", fmt.Errorf("failed to save payment to DB: %w", err)
	}

	log.Printf("Payment created: DB ID=%s, YooKassa ID=%s, amount=%.2f, tax=%.2f",
		paymentID, result.ID, amount, taxAmount)

	return result.Confirmation.ConfirmationURL, paymentID, nil
}

func (s *YooKassaService) HandleWebhook(ctx context.Context, db *sql.DB, body []byte, signature string) error {
	if s.webhookSecret != "" {
		if !s.verifySignature(body, signature) {
			return fmt.Errorf("invalid webhook signature")
		}
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal webhook: %w", err)
	}

	log.Printf("Webhook received: type=%s, event=%s", payload.Type, payload.Event)

	if payload.Type != "notification" || payload.Event != "payment.succeeded" {
		log.Printf("Webhook ignored: not a successful payment notification")
		return nil
	}

	var obj PaymentObject
	if err := json.Unmarshal(payload.Object, &obj); err != nil {
		return fmt.Errorf("failed to unmarshal payment object: %w", err)
	}

	log.Printf("Payment event parsed: ID=%s, Status=%s, Paid=%v", obj.ID, obj.Status, obj.Paid)

	if obj.Status == "succeeded" || obj.Paid {
		now := time.Now()
		var amount, taxAmount float64

		query := `
			UPDATE payments 
			SET status = 'succeeded', paid_at = $1
			WHERE yookassa_id = $2 AND status = 'pending'
			RETURNING amount, tax_amount
		`

		err := db.QueryRowContext(ctx, query, now, obj.ID).Scan(&amount, &taxAmount)
		if err != nil {
			return fmt.Errorf("failed to update payment: %w", err)
		}

		log.Printf("Payment updated in DB: YooKassa ID=%s, amount=%.2f, tax=%.2f", obj.ID, amount, taxAmount)

		if err := s.updateTaxReport(ctx, db, now, taxAmount); err != nil {
			log.Printf("Warning: failed to update tax report: %v", err)
		}
	}

	return nil
}

func (s *YooKassaService) verifySignature(body []byte, signature string) bool {
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.webhookSecret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (s *YooKassaService) updateTaxReport(ctx context.Context, db *sql.DB, paidAt time.Time, taxAmount float64) error {
	monthStart := time.Date(paidAt.Year(), paidAt.Month(), 1, 0, 0, 0, 0, paidAt.Location())

	query := `
		INSERT INTO tax_reports (month, total_income, total_tax, payments_count)
		VALUES ($1, 0, 0, 0)
		ON CONFLICT (month) DO NOTHING
	`
	db.ExecContext(ctx, query, monthStart)

	// ИСПРАВЛЕНО: явное приведение типов для PostgreSQL
	query = `
		UPDATE tax_reports 
		SET total_income = total_income + ($1::numeric) / ($2::numeric),
		    total_tax = total_tax + $1::numeric,
		    payments_count = payments_count + 1
		WHERE month = $3
	`
	_, err := db.ExecContext(ctx, query, taxAmount, 0.04, monthStart)
	return err
}

func (s *YooKassaService) GetPaymentStatus(ctx context.Context, db *sql.DB, paymentID string) (*PaymentRecord, error) {
	log.Printf("DEBUG GetPaymentStatus: Querying for ID = '%s'", paymentID)
	
	query := `
		SELECT id, session_id, yookassa_id, amount, status, spread_name, 
		       tax_amount, tax_rate, created_at, paid_at, receipt_url
		FROM payments WHERE id = $1
	`

	var rec PaymentRecord
	var paidAt sql.NullTime

	err := db.QueryRowContext(ctx, query, paymentID).Scan(
		&rec.ID, &rec.SessionID, &rec.YooKassaID, &rec.Amount, &rec.Status,
		&rec.SpreadName, &rec.TaxAmount, &rec.TaxRate, &rec.CreatedAt, &paidAt, &rec.ReceiptURL,
	)

	if err != nil {
		log.Printf("ERROR GetPaymentStatus: DB query failed for ID '%s': %v", paymentID, err)
		return nil, err
	}

	if paidAt.Valid {
		rec.PaidAt = &paidAt.Time
	}

	return &rec, nil
}

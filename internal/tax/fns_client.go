package tax

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type FNSClient struct {
	username     string
	password     string
	deviceID     string
	baseURL      string
	httpClient   *http.Client
	db           *sql.DB

	accessToken  string
	refreshToken string
	tokenExpires time.Time
}

type authRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	DeviceInfo struct {
		SourceDeviceID string `json:"sourceDeviceId"`
		SourceType     string `json:"sourceType"`
		AppVersion     string `json:"appVersion"`
		MetaDetails    struct {
			UserAgent string `json:"userAgent"`
		} `json:"metaDetails"`
	} `json:"deviceInfo"`
}

type authResponse struct {
	Token         string `json:"token"`
	RefreshToken  string `json:"refreshToken"`
	TokenExpireIn string `json:"tokenExpireIn"`
}

// incomeClient — структура клиента (обязательна для ФНС)
type incomeClient struct {
	ContactPhone *string `json:"contactPhone"`
	DisplayName  *string `json:"displayName"`
	IncomeType   string  `json:"incomeType"` // "FROM_INDIVIDUAL"
	INN          *string `json:"inn"`
}

// incomeService — структура услуги (сумма СТРОКОЙ!)
type incomeService struct {
	Name     string `json:"name"`
	Amount   string `json:"amount"`
	Quantity int    `json:"quantity"`
}

// incomeRequest — точная структура, которую принимает ФНС
type incomeRequest struct {
	PaymentType                   string          `json:"paymentType"` // "CASH"
	IgnoreMaxTotalIncomeRestriction bool          `json:"ignoreMaxTotalIncomeRestriction"`
	Client                        incomeClient    `json:"client"`
	RequestTime                   string          `json:"requestTime"`
	OperationTime                 string          `json:"operationTime"`
	Services                      []incomeService `json:"services"`
	TotalAmount                   string          `json:"totalAmount"` // СТРОКА!
}

type incomeResponse struct {
	UID    string `json:"approvedReceiptUuid"`
	Status string `json:"status"`
}

func NewFNSClient(db *sql.DB) (*FNSClient, error) {
	username := os.Getenv("FNS_LOGIN")
	password := os.Getenv("FNS_PASSWORD")
	deviceID := os.Getenv("FNS_DEVICE_ID")
	baseURL := strings.TrimRight(os.Getenv("FNS_BASE_URL"), "/")

	if baseURL == "" {
		baseURL = "https://lknpd.nalog.ru/api/v1"
	}

	if username == "" || password == "" || deviceID == "" {
		return nil, fmt.Errorf("FNS_LOGIN (ИНН), FNS_PASSWORD и FNS_DEVICE_ID обязательны")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	client := &FNSClient{
		username:   username,
		password:   password,
		deviceID:   deviceID,
		baseURL:    baseURL,
		httpClient: httpClient,
		db:         db,
	}

	if err := client.loadTokenFromDB(); err != nil {
		log.Printf("FNS: Token not found in DB, will authenticate on first use: %v", err)
	}

	return client, nil
}

func (c *FNSClient) loadTokenFromDB() error {
	query := `SELECT access_token, refresh_token, token_expires_at FROM self_employed_auth WHERE id = 1`
	var accessToken, refreshToken string
	var expiresAt time.Time

	err := c.db.QueryRow(query).Scan(&accessToken, &refreshToken, &expiresAt)
	if err != nil {
		return err
	}

	c.accessToken = accessToken
	c.refreshToken = refreshToken
	c.tokenExpires = expiresAt

	log.Printf("FNS: Loaded token from DB, expires at %s", expiresAt.Format(time.RFC3339))
	return nil
}

func (c *FNSClient) saveTokenToDB() error {
	query := `
		INSERT INTO self_employed_auth (id, access_token, refresh_token, token_expires_at, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE
		SET access_token = EXCLUDED.access_token,
		    refresh_token = EXCLUDED.refresh_token,
		    token_expires_at = EXCLUDED.token_expires_at,
		    updated_at = NOW()
	`
	_, err := c.db.Exec(query, c.accessToken, c.refreshToken, c.tokenExpires)
	return err
}

func (c *FNSClient) Authenticate() error {
	authURL := c.baseURL + "/auth/lkfl"
	log.Printf("FNS: Authenticating. URL=%s, username=%s", authURL, c.username)

	reqBody := authRequest{
		Username: c.username,
		Password: c.password,
	}
	reqBody.DeviceInfo.SourceDeviceID = c.deviceID
	reqBody.DeviceInfo.SourceType = "WEB"
	reqBody.DeviceInfo.AppVersion = "1.0.0"
	reqBody.DeviceInfo.MetaDetails.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 11_2_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.192 Safari/537.36"

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequest("POST", authURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://lknpd.nalog.ru/")
	req.Header.Set("Origin", "https://lknpd.nalog.ru")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 11_2_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.192 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp authResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w, body: %s", err, string(body))
	}

	c.accessToken = authResp.Token
	c.refreshToken = authResp.RefreshToken
	
	expireTime, err := time.Parse(time.RFC3339, authResp.TokenExpireIn)
	if err != nil {
		log.Printf("FNS: Warning: failed to parse token expiration time: %v", err)
		c.tokenExpires = time.Now().Add(24 * time.Hour)
	} else {
		c.tokenExpires = expireTime
	}

	if err := c.saveTokenToDB(); err != nil {
		return fmt.Errorf("failed to save token to DB: %w", err)
	}

	log.Printf("FNS: Authentication successful, token expires at %s", c.tokenExpires.Format(time.RFC3339))
	return nil
}

func (c *FNSClient) RefreshToken() error {
	log.Printf("FNS: Token expired or missing, performing full re-authentication")
	return c.Authenticate()
}

func (c *FNSClient) EnsureValidToken() error {
	if c.accessToken == "" || time.Until(c.tokenExpires) < time.Hour {
		return c.RefreshToken()
	}
	return nil
}

func (c *FNSClient) RegisterIncome(amount float64, serviceName, operationTime string) (string, error) {
	if err := c.EnsureValidToken(); err != nil {
		return "", fmt.Errorf("failed to ensure valid token: %w", err)
	}

	currentTime := time.Now().Format(time.RFC3339)
	amountStr := fmt.Sprintf("%.2f", amount) // Сумма СТРОКОЙ

	reqBody := incomeRequest{
		PaymentType:                   "CASH", // Работает гарантированно для физлиц
		IgnoreMaxTotalIncomeRestriction: false,
		Client: incomeClient{
			ContactPhone: nil,
			DisplayName:  nil,
			IncomeType:   "FROM_INDIVIDUAL", // ВАЖНО: не clientType на корню!
			INN:          nil,
		},
		RequestTime:   currentTime,
		OperationTime: currentTime,
		TotalAmount:   amountStr, // СТРОКА
		Services: []incomeService{
			{
				Name:     serviceName,
				Amount:   amountStr, // СТРОКА
				Quantity: 1,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal income request: %w", err)
	}
	
	log.Printf("FNS: Income request JSON payload: %s", string(jsonData))

	incomeURL := c.baseURL + "/income"
	req, err := http.NewRequest("POST", incomeURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Referer", "https://lknpd.nalog.ru/sales/create")
	req.Header.Set("Origin", "https://lknpd.nalog.ru")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 11_2_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.192 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("income request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("income registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	var incomeResp incomeResponse
	if err := json.Unmarshal(body, &incomeResp); err != nil {
		return "", fmt.Errorf("failed to decode income response: %w, body: %s", err, string(body))
	}

	log.Printf("FNS: Income registered successfully. UID=%s, Status=%s, Amount=%s", incomeResp.UID, incomeResp.Status, amountStr)
	return incomeResp.UID, nil
}

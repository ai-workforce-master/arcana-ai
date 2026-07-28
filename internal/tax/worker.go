package tax

import (
	"context"
	"database/sql"
	"log"
	"time"
)

type Worker struct {
	db     *sql.DB
	client *FNSClient
}

func NewWorker(db *sql.DB, client *FNSClient) *Worker {
	return &Worker{db: db, client: client}
}

// Start запускает фоновую обработку очереди чеков
func (w *Worker) Start(ctx context.Context) {
	log.Println("FNS Worker: Starting background receipt processor...")

	// Пробуем авторизоваться при старте, чтобы сразу выявить проблемы с логином/паролем
	if err := w.client.Authenticate(); err != nil {
		log.Printf("FNS Worker: Initial authentication failed: %v. Will retry on first receipt.", err)
	} else {
		log.Println("FNS Worker: Initial authentication successful. Token saved.")
	}

	// Проверяем очередь каждые 30 секунд
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("FNS Worker: Stopping background processor.")
			return
		case <-ticker.C:
			w.processPendingReceipts()
		}
	}
}

func (w *Worker) processPendingReceipts() {
	// Берём до 10 чеков со статусом pending и менее чем 5 попытками
	query := `
		SELECT id, amount, service_name, operation_time 
		FROM receipts 
		WHERE status = 'pending' AND attempts < 5 
		ORDER BY created_at ASC 
		LIMIT 10
	`
	rows, err := w.db.Query(query)
	if err != nil {
		log.Printf("FNS Worker: Failed to query pending receipts: %v", err)
		return
	}
	defer rows.Close()

	type receipt struct {
		id            string
		amount        float64
		serviceName   string
		operationTime time.Time
	}

	var receiptsToProcess []receipt
	for rows.Next() {
		var r receipt
		if err := rows.Scan(&r.id, &r.amount, &r.serviceName, &r.operationTime); err != nil {
			log.Printf("FNS Worker: Failed to scan receipt: %v", err)
			continue
		}
		receiptsToProcess = append(receiptsToProcess, r)
	}

	if len(receiptsToProcess) == 0 {
		return // Нечего обрабатывать
	}

	log.Printf("FNS Worker: Found %d pending receipts to process", len(receiptsToProcess))

	for _, r := range receiptsToProcess {
		log.Printf("FNS Worker: Processing receipt %s for amount %.2f (%s)", r.id, r.amount, r.serviceName)

		// Сразу увеличиваем счётчик попыток, чтобы избежать бесконечных циклов при сбоях
		_, err = w.db.Exec(`UPDATE receipts SET attempts = attempts + 1 WHERE id = $1`, r.id)
		if err != nil {
			log.Printf("FNS Worker: Failed to increment attempts for receipt %s: %v", r.id, err)
			continue
		}

		opTimeStr := r.operationTime.Format(time.RFC3339)
		
		// RegisterIncome автоматически вызовет EnsureValidToken() и обновит токен при необходимости
		uid, err := w.client.RegisterIncome(r.amount, r.serviceName, opTimeStr)

		if err != nil {
			log.Printf("FNS Worker: Failed to register income for receipt %s: %v", r.id, err)
			
			// Проверяем, достигли ли мы лимита попыток
			var attempts int
			w.db.QueryRow(`SELECT attempts FROM receipts WHERE id = $1`, r.id).Scan(&attempts)
			
			if attempts >= 5 {
				_, updateErr := w.db.Exec(`
					UPDATE receipts 
					SET status = 'failed', error_message = $1 
					WHERE id = $2
				`, err.Error(), r.id)
				if updateErr != nil {
					log.Printf("FNS Worker: Failed to mark receipt %s as failed: %v", r.id, updateErr)
				} else {
					log.Printf("FNS Worker: Marked receipt %s as FAILED after %d attempts", r.id, attempts)
				}
			}
		} else {
			// Успех!
			_, updateErr := w.db.Exec(`
				UPDATE receipts 
				SET status = 'sent', fns_uid = $1, sent_at = NOW() 
				WHERE id = $2
			`, uid, r.id)
			
			if updateErr != nil {
				log.Printf("FNS Worker: Failed to update receipt %s as sent: %v", r.id, updateErr)
			} else {
				log.Printf("FNS Worker: ✅ Successfully sent receipt %s to FNS (UID: %s)", r.id, uid)
			}
		}

		// Небольшая пауза между запросами, чтобы не перегружать API ФНС
		time.Sleep(1 * time.Second)
	}
}

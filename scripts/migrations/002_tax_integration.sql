-- Таблица для хранения OAuth-токенов API «Мой налог»
-- Всегда одна запись (id=1), обновляется при каждом refresh
CREATE TABLE IF NOT EXISTS self_employed_auth (
    id               INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    access_token     TEXT NOT NULL,
    refresh_token    TEXT NOT NULL,
    token_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица очереди чеков на отправку в ФНС
CREATE TABLE IF NOT EXISTS receipts (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id     UUID REFERENCES payments(id) ON DELETE SET NULL,
    amount         NUMERIC(10, 2) NOT NULL,
    service_name   VARCHAR(255) NOT NULL,
    operation_time TIMESTAMP WITH TIME ZONE NOT NULL,
    status         VARCHAR(50) NOT NULL DEFAULT 'pending',
    -- pending = в очереди, sent = отправлен, failed = ошибка после ретраев
    fns_uid        VARCHAR(255) UNIQUE,
    error_message  TEXT,
    attempts       INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    sent_at        TIMESTAMP WITH TIME ZONE
);

-- Индексы для ускорения выборки очереди и поиска по платежу
CREATE INDEX IF NOT EXISTS idx_receipts_status ON receipts(status);
CREATE INDEX IF NOT EXISTS idx_receipts_payment_id ON receipts(payment_id);
CREATE INDEX IF NOT EXISTS idx_receipts_fns_uid ON receipts(fns_uid);

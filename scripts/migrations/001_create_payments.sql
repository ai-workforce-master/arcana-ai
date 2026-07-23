-- Таблица платежей
CREATE TABLE IF NOT EXISTS payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      VARCHAR(255) NOT NULL,           -- ID сессии пользователя (анонимный)
    yookassa_id     VARCHAR(255) UNIQUE NOT NULL,    -- ID платежа в ЮKassa
    amount          NUMERIC(10, 2) NOT NULL,         -- Сумма платежа (руб)
    currency        VARCHAR(3) DEFAULT 'RUB',
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending, succeeded, canceled, refunded
    spread_name     VARCHAR(255),                    -- Название расклада
    question_hash   VARCHAR(255),                    -- Хэш вопроса (для аналитики, без текста)
    
    -- Налоговый учёт
    tax_rate        NUMERIC(3, 2) DEFAULT 0.04,     -- Ставка налога (4% для физлиц)
    tax_amount      NUMERIC(10, 2),                  -- Сумма налога (amount * tax_rate)
    tax_paid        BOOLEAN DEFAULT FALSE,           -- Оплачен ли налог
    tax_paid_at     TIMESTAMP WITH TIME ZONE,
    
    -- Временные метки
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    paid_at         TIMESTAMP WITH TIME ZONE,        -- Когда подтверждена оплата
    receipt_url     TEXT,                            -- Ссылка на чек
    
    -- Метаданные
    metadata        JSONB DEFAULT '{}',
    idempotency_key VARCHAR(255) UNIQUE
);

-- Индексы для быстрого поиска
CREATE INDEX idx_payments_yookassa_id ON payments(yookassa_id);
CREATE INDEX idx_payments_session_id ON payments(session_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_created_at ON payments(created_at);
CREATE INDEX idx_payments_tax_paid ON payments(tax_paid);

-- Таблица для агрегированного налогового учёта (месячные сводки)
CREATE TABLE IF NOT EXISTS tax_reports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    month           DATE NOT NULL,                   -- Первый день месяца (2026-07-01)
    total_income    NUMERIC(12, 2) NOT NULL,         -- Общий доход за месяц
    total_tax       NUMERIC(12, 2) NOT NULL,         -- Общий налог за месяц
    payments_count  INTEGER NOT NULL,
    tax_paid        BOOLEAN DEFAULT FALSE,
    tax_paid_at     TIMESTAMP WITH TIME ZONE,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(month)
);

CREATE INDEX idx_tax_reports_month ON tax_reports(month);

CREATE TABLE IF NOT EXISTS payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      VARCHAR(255) NOT NULL,
    yookassa_id     VARCHAR(255) UNIQUE NOT NULL,
    amount          NUMERIC(10, 2) NOT NULL,
    currency        VARCHAR(3) DEFAULT 'RUB',
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    spread_name     VARCHAR(255),
    tax_amount      NUMERIC(10, 2),
    tax_rate        NUMERIC(3, 2) DEFAULT 0.04,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    paid_at         TIMESTAMP WITH TIME ZONE,
    receipt_url     TEXT,
    idempotency_key VARCHAR(255) UNIQUE
);

CREATE TABLE IF NOT EXISTS tax_reports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    month           DATE NOT NULL,
    total_income    NUMERIC(12, 2) NOT NULL,
    total_tax       NUMERIC(12, 2) NOT NULL,
    payments_count  INTEGER NOT NULL,
    tax_paid        BOOLEAN DEFAULT FALSE,
    tax_paid_at     TIMESTAMP WITH TIME ZONE,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(month)
);

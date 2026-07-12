PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    login TEXT NOT NULL UNIQUE COLLATE NOCASE,
    created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    base_day_cost INTEGER NOT NULL DEFAULT 0,
    horizon_policy TEXT NOT NULL DEFAULT 'END_OF_MONTH',
    horizon_custom_date TEXT,
    currency TEXT NOT NULL DEFAULT 'RUB',
    updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS anchors (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount INTEGER NOT NULL,
    anchored_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_anchors_user ON anchors(user_id, anchored_at DESC, created_at DESC);
CREATE TABLE IF NOT EXISTS categories (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('INCOME','EXPENSE')),
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    UNIQUE(user_id, name, kind)
);
CREATE TABLE IF NOT EXISTS financial_events (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('INCOME','EXPENSE')),
    necessity TEXT NOT NULL CHECK(necessity IN ('REQUIRED','OPTIONAL')),
    amount INTEGER NOT NULL CHECK(amount > 0),
    category_id TEXT REFERENCES categories(id) ON DELETE SET NULL,
    comment TEXT NOT NULL DEFAULT '',
    recurrence_type TEXT NOT NULL CHECK(recurrence_type IN ('NONE','MONTHLY_ON_DAY','EVERY_N_DAYS')),
    start_date TEXT NOT NULL,
    day_of_month INTEGER,
    interval_days INTEGER,
    until_date TEXT,
    cron_expr TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_user ON financial_events(user_id);
CREATE TABLE IF NOT EXISTS day_cost_overrides (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date_from TEXT NOT NULL,
    date_to TEXT NOT NULL,
    daily_amount INTEGER NOT NULL CHECK(daily_amount >= 0),
    comment TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    CHECK(date_from <= date_to)
);
CREATE INDEX IF NOT EXISTS idx_day_cost_user ON day_cost_overrides(user_id, date_from, date_to);
CREATE TABLE IF NOT EXISTS event_occurrence_overrides (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES financial_events(id) ON DELETE CASCADE,
    occurrence_date TEXT NOT NULL,
    amount INTEGER,
    comment TEXT,
    is_skipped INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    UNIQUE(event_id, occurrence_date)
);
CREATE TABLE IF NOT EXISTS actual_transactions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('INCOME','EXPENSE')),
    amount INTEGER NOT NULL CHECK(amount > 0),
    category_id TEXT REFERENCES categories(id) ON DELETE SET NULL,
    transaction_date TEXT NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    event_id TEXT REFERENCES financial_events(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE migration_money_guard (valid INTEGER CONSTRAINT legacy_money_out_of_range CHECK(valid=1));
INSERT INTO migration_money_guard SELECT 0 WHERE
 EXISTS(SELECT 1 FROM anchors WHERE amount<0 OR amount>9007199254740991) OR
 EXISTS(SELECT 1 FROM settings WHERE base_day_cost<0 OR base_day_cost>9007199254740991) OR
 EXISTS(SELECT 1 FROM financial_events WHERE amount<1 OR amount>9007199254740991) OR
 EXISTS(SELECT 1 FROM actual_transactions WHERE amount<1 OR amount>9007199254740991) OR
 EXISTS(SELECT 1 FROM day_cost_overrides WHERE daily_amount<0 OR daily_amount>9007199254740991) OR
 EXISTS(SELECT 1 FROM event_occurrence_overrides WHERE amount IS NOT NULL AND (amount<1 OR amount>9007199254740991));
DROP TABLE migration_money_guard;

-- Legacy transactions carry only a Moscow financial date. They remain
-- standalone facts; an event_id is not enough to infer an occurrence key.
CREATE TABLE legacy_facts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    effective_at DATETIME NOT NULL,
    sequence INTEGER NOT NULL UNIQUE,
    kind TEXT NOT NULL,
    amount INTEGER NOT NULL,
    category_id TEXT,
    comment TEXT NOT NULL
);
INSERT INTO legacy_facts SELECT id,user_id,datetime(transaction_date || ' 09:00:00'),
    row_number() OVER(ORDER BY rowid),kind,amount,category_id,comment FROM actual_transactions;
CREATE INDEX idx_legacy_facts_user ON legacy_facts(user_id,effective_at,sequence);

CREATE TABLE ledger_clock (id INTEGER PRIMARY KEY CHECK(id = 1), value INTEGER NOT NULL);
INSERT INTO ledger_clock VALUES(1, (SELECT count(*) FROM anchors) + (SELECT count(*) FROM legacy_facts));
ALTER TABLE anchors ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0;
WITH ranked AS (
    SELECT id, (SELECT count(*) FROM legacy_facts) + row_number() OVER (ORDER BY julianday(anchored_at), julianday(created_at), rowid) AS seq
    FROM anchors
)
UPDATE anchors SET sequence = (SELECT seq FROM ranked WHERE ranked.id = anchors.id);
CREATE UNIQUE INDEX idx_anchors_sequence ON anchors(sequence);

ALTER TABLE settings ADD COLUMN needs_balance_review INTEGER NOT NULL DEFAULT 0;
UPDATE settings SET needs_balance_review = 1 WHERE EXISTS (
    SELECT 1 FROM anchors WHERE anchors.user_id = settings.user_id
);
ALTER TABLE financial_events ADD COLUMN deleted_at DATETIME;

CREATE TABLE occurrence_states (
    event_id TEXT NOT NULL REFERENCES financial_events(id),
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    occurrence_date TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('CONFIRMED', 'CANCELLED')),
    effective_at DATETIME,
    sequence INTEGER,
    kind TEXT NOT NULL CHECK(kind IN ('INCOME','EXPENSE')),
    necessity TEXT NOT NULL CHECK(necessity IN ('REQUIRED','OPTIONAL')),
    amount INTEGER NOT NULL CHECK(amount > 0),
    category_id TEXT,
    comment TEXT NOT NULL,
    counts_toward_day_budget INTEGER NOT NULL DEFAULT 0 CHECK(counts_toward_day_budget IN (0,1)),
    created_at DATETIME NOT NULL,
    PRIMARY KEY(event_id, occurrence_date),
    CHECK(counts_toward_day_budget = 0 OR kind = 'EXPENSE'),
    CHECK((status = 'CONFIRMED' AND effective_at IS NOT NULL AND sequence IS NOT NULL)
       OR (status = 'CANCELLED' AND effective_at IS NULL AND sequence IS NULL))
);
CREATE INDEX idx_occurrences_user ON occurrence_states(user_id, effective_at);
CREATE UNIQUE INDEX idx_occurrences_sequence ON occurrence_states(sequence) WHERE sequence IS NOT NULL;

CREATE TABLE occurrence_unconfirmations (
    event_id TEXT NOT NULL REFERENCES financial_events(id),
    occurrence_date TEXT NOT NULL,
    PRIMARY KEY(event_id,occurrence_date)
);

CREATE TABLE base_day_cost_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    effective_from TEXT NOT NULL,
    amount INTEGER NOT NULL CHECK(amount >= 0)
);
-- Legacy only knows the last saved value, not the dates of earlier changes.
INSERT INTO base_day_cost_history(user_id,effective_from,amount)
SELECT user_id,'0001-01-01',base_day_cost FROM settings;
CREATE INDEX idx_day_cost_history_user ON base_day_cost_history(user_id,effective_from,id);

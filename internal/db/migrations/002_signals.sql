-- up

-- Signals (go-to-market signals detected about a person or organization)
CREATE TABLE IF NOT EXISTS signals (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT NOT NULL UNIQUE,
    signal_type TEXT NOT NULL,
    description TEXT,
    person_id   INTEGER REFERENCES people(id),
    org_id      INTEGER REFERENCES organizations(id),
    detected_at TEXT NOT NULL DEFAULT (datetime('now')),
    archived    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_signals_org      ON signals(org_id)      WHERE archived = 0;
CREATE INDEX IF NOT EXISTS idx_signals_person   ON signals(person_id)   WHERE archived = 0;
CREATE INDEX IF NOT EXISTS idx_signals_type     ON signals(signal_type) WHERE archived = 0;
CREATE INDEX IF NOT EXISTS idx_signals_detected ON signals(detected_at) WHERE archived = 0;

-- down

DROP TABLE IF EXISTS signals;

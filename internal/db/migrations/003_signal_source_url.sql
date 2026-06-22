-- up
ALTER TABLE signals ADD COLUMN source_url TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS ux_signals_source_url
    ON signals(source_url) WHERE archived = 0 AND source_url IS NOT NULL;

-- down
DROP INDEX IF EXISTS ux_signals_source_url;
ALTER TABLE signals DROP COLUMN source_url;

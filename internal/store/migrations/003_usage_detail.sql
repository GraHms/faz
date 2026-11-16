-- Per call detail for telemetry: how hard the model was asked to think,
-- which try this was, how long it took, and why it failed.
ALTER TABLE model_usage
    ADD COLUMN effort     text NOT NULL DEFAULT '',
    ADD COLUMN attempt    int  NOT NULL DEFAULT 1,
    ADD COLUMN latency_ms int  NOT NULL DEFAULT 0,
    ADD COLUMN error      text;
CREATE INDEX model_usage_created ON model_usage (created_at);

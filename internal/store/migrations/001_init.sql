-- Faz product schema. Money is stored in whole meticais and whole credits.

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    name          text NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Only a hash of each session token is stored.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user ON sessions (user_id);

-- A small Postgres job queue claimed with FOR UPDATE SKIP LOCKED.
CREATE TABLE jobs (
    id           bigserial PRIMARY KEY,
    kind         text NOT NULL,
    payload      jsonb NOT NULL,
    status       text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    attempts     int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 2,
    run_after    timestamptz NOT NULL DEFAULT now(),
    locked_at    timestamptz,
    locked_by    text,
    last_error   text,
    result       jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_ready ON jobs (run_after, id) WHERE status = 'queued';
CREATE INDEX jobs_running ON jobs (locked_at) WHERE status = 'running';

CREATE TABLE sites (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    business_name        text NOT NULL,
    business_type        text NOT NULL,
    prompt               text NOT NULL,
    status               text NOT NULL CHECK (status IN ('questions', 'building', 'ready', 'editing', 'failed')),
    brief                jsonb,
    answers              jsonb,
    current_version      int NOT NULL DEFAULT 0,
    published_version    int,
    subdomain            text UNIQUE,
    published_at         timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sites_user ON sites (user_id, created_at DESC);

-- Every build or kept edit is a version; its files live under r2_prefix.
CREATE TABLE site_versions (
    site_id     uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    number      int NOT NULL,
    kind        text NOT NULL CHECK (kind IN ('build', 'edit')),
    r2_prefix   text NOT NULL,
    status      text NOT NULL,
    instruction text,
    job_id      bigint REFERENCES jobs(id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, number)
);

CREATE TABLE site_assets (
    site_id    uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name       text NOT NULL,
    media_type text NOT NULL,
    size       bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, name)
);

CREATE TABLE messages (
    id              bigserial PRIMARY KEY,
    site_id         uuid NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('user', 'assistant')),
    text            text NOT NULL,
    options         jsonb,
    job_id          bigint REFERENCES jobs(id),
    credits_charged int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX messages_site ON messages (site_id, id);

CREATE TABLE payments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    method         text NOT NULL CHECK (method IN ('mpesa', 'emola')),
    msisdn         text NOT NULL,
    amount_mzn     int NOT NULL CHECK (amount_mzn > 0),
    vat_mzn        int NOT NULL,
    credits        int NOT NULL,
    nuit           text,
    status         text NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed')),
    provider       text NOT NULL,
    provider_ref   text,
    failure_reason text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payments_user ON payments (user_id, created_at DESC);
CREATE UNIQUE INDEX payments_provider_ref ON payments (provider, provider_ref) WHERE provider_ref IS NOT NULL;

-- Append only. A balance is the sum of deltas. Idempotency keys make
-- refunds and top ups safe to retry.
CREATE TABLE credit_ledger (
    id              bigserial PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delta           int NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('signup_gift', 'topup', 'charge', 'refund', 'adjustment')),
    reason          text NOT NULL,
    job_id          bigint REFERENCES jobs(id),
    payment_id      uuid REFERENCES payments(id),
    idempotency_key text UNIQUE,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX credit_ledger_user ON credit_ledger (user_id, id);

-- Model spend per stage, next to what was charged, so margins are measured.
CREATE TABLE model_usage (
    id            bigserial PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    site_id       uuid REFERENCES sites(id) ON DELETE SET NULL,
    job_id        bigint REFERENCES jobs(id),
    operation     text NOT NULL,
    stage         text NOT NULL,
    model         text NOT NULL,
    input_tokens  int NOT NULL,
    output_tokens int NOT NULL,
    cache_read    int NOT NULL,
    cache_write   int NOT NULL,
    cost_usd      numeric(12, 6) NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX model_usage_site ON model_usage (site_id);

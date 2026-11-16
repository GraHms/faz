-- Operator controls: runtime settings, beta invite codes, an audit log of
-- every admin action, and flags for suspended accounts and taken down sites.

CREATE TABLE settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invite_codes (
    code          text PRIMARY KEY CHECK (code ~ '^[A-Z0-9][A-Z0-9-]{2,39}$'),
    note          text NOT NULL DEFAULT '',
    max_uses      int  NOT NULL CHECK (max_uses > 0),
    uses          int  NOT NULL DEFAULT 0 CHECK (uses >= 0),
    bonus_credits int  NOT NULL DEFAULT 0 CHECK (bonus_credits >= 0),
    expires_at    timestamptz,
    disabled_at   timestamptz,
    created_by    text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (uses <= max_uses)
);

CREATE TABLE admin_audit (
    id          bigserial PRIMARY KEY,
    operator    text NOT NULL,
    action      text NOT NULL,
    target_type text NOT NULL,
    target_id   text NOT NULL,
    reason      text NOT NULL,
    details     jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX admin_audit_target ON admin_audit (target_type, target_id);

ALTER TABLE users
    ADD COLUMN invite_code      text REFERENCES invite_codes(code),
    ADD COLUMN suspended_at     timestamptz,
    ADD COLUMN suspended_reason text;

ALTER TABLE sites
    ADD COLUMN taken_down_at   timestamptz,
    ADD COLUMN takedown_reason text;

CREATE INDEX payments_msisdn ON payments (msisdn);

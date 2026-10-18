CREATE TABLE IF NOT EXISTS sub_adds_recharge_orders (
 id varchar(32) PRIMARY KEY, user_id bigint NOT NULL, package_id text NOT NULL,
 amount_cents bigint NOT NULL CHECK(amount_cents > 0), credit numeric(20,8) NOT NULL CHECK(credit > 0),
 payment_type text NOT NULL, status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','paid')),
 trade_no text UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), paid_at timestamptz,
 cache_synced boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS sub_adds_recharge_user ON sub_adds_recharge_orders(user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS sub_adds_recharge_logs (
 id bigserial PRIMARY KEY, order_id varchar(32) NOT NULL REFERENCES sub_adds_recharge_orders(id),
 event text NOT NULL, detail text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);

-- Additive migrations also run on installations created before reconciliation support.
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS last_checked_at timestamptz;
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS next_check_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS gateway_status text NOT NULL DEFAULT '';
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS query_failures integer NOT NULL DEFAULT 0;
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS reversed_at timestamptz;
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS refund_reference text;
CREATE UNIQUE INDEX IF NOT EXISTS sub_adds_recharge_refund_ref ON sub_adds_recharge_orders(refund_reference) WHERE refund_reference IS NOT NULL;
CREATE INDEX IF NOT EXISTS sub_adds_recharge_next_check ON sub_adds_recharge_orders(next_check_at);
CREATE TABLE IF NOT EXISTS sub_adds_recharge_issues (
 id bigserial PRIMARY KEY, order_id varchar(32) NOT NULL REFERENCES sub_adds_recharge_orders(id),
 kind text NOT NULL, detail text NOT NULL, occurrences bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 resolved_at timestamptz, resolved_by bigint, resolution text NOT NULL DEFAULT '',
 notified_at timestamptz, notify_after timestamptz NOT NULL DEFAULT now(), notify_attempts integer NOT NULL DEFAULT 0,
 UNIQUE(order_id, kind)
);
CREATE INDEX IF NOT EXISTS sub_adds_recharge_open_issues ON sub_adds_recharge_issues(updated_at DESC) WHERE resolved_at IS NULL;
ALTER TABLE sub_adds_recharge_issues ADD COLUMN IF NOT EXISTS alert_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS cache_version bigint NOT NULL DEFAULT 0;

-- Existing redirect-based orders are kept as legacy; only new orders may call mapi.php.
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS request_key text;
CREATE UNIQUE INDEX IF NOT EXISTS sub_adds_recharge_request_key ON sub_adds_recharge_orders(user_id,request_key) WHERE request_key IS NOT NULL;
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS payment_state text NOT NULL DEFAULT 'legacy';
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS payment_requested_at timestamptz;
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS payment_source text NOT NULL DEFAULT '';
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS payment_source_kind text NOT NULL DEFAULT '';
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS payment_qr text NOT NULL DEFAULT '';
ALTER TABLE sub_adds_recharge_orders ADD COLUMN IF NOT EXISTS payment_qr_kind text NOT NULL DEFAULT '';

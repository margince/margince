-- Put the table back exactly as the catalog held it, defaults, key and index
-- and all. The rows are not restored: nothing read them, and audit_log carries
-- the same changes.
SET LOCAL lock_timeout = '3s';

CREATE TABLE deal_forecast_history (
    id uuid DEFAULT uuidv7() NOT NULL,
    deal_id uuid NOT NULL,
    changed_by text NOT NULL,
    changed_at timestamptz DEFAULT now() NOT NULL,
    amount_minor_at_change bigint,
    currency_at_change char(3),
    close_date_at_change date
);

ALTER TABLE deal_forecast_history
    ADD CONSTRAINT deal_forecast_history_pkey PRIMARY KEY (id);

ALTER TABLE deal_forecast_history
    ADD CONSTRAINT deal_forecast_history_deal_id_fkey
        FOREIGN KEY (deal_id) REFERENCES deal(id) ON DELETE CASCADE;

CREATE INDEX idx_deal_forecast_history_deal
    ON deal_forecast_history USING btree (deal_id, changed_at);

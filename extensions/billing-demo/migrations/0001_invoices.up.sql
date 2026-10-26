-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- The demonstration billing system's own books: the connection an admin made
-- (which service member the feed runs as, and which pipeline and stages its deals
-- use) and the invoices the feed mirrors into deals. Nothing here is a credential.

CREATE TABLE ext.ext_billing_demo_connection (
    only_row        boolean     NOT NULL DEFAULT true PRIMARY KEY CHECK (only_row),
    member_id       uuid        NOT NULL,
    pipeline_id     uuid        NOT NULL,
    open_stage_id   uuid        NOT NULL,
    won_stage_id    uuid        NOT NULL,
    connected_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ext.ext_billing_demo_invoice (
    erp_id              text        NOT NULL PRIMARY KEY CHECK (length(erp_id) > 0),
    name                text        NOT NULL,
    currency            text        NOT NULL,
    amount_minor        bigint      NOT NULL CHECK (amount_minor >= 0),
    paid                boolean     NOT NULL DEFAULT false,
    -- What the core last answered for this invoice. The feed offers the invoice
    -- again whenever its amount or paid flag differs from what was synced.
    synced_amount_minor bigint,
    synced_paid         boolean,
    deal_id             uuid,
    -- The fields the core left as they were because a member edited them.
    withheld            text[]      NOT NULL DEFAULT '{}',
    synced_at           timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_billing_demo_connection TO margince_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_billing_demo_invoice TO margince_app;
  END IF;
END $$;

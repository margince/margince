-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The existing-customer route (UWG §7(3)) needs proof that a customer was told
-- at collection that they may object to advertising. An admin declares it once
-- per customer group: the sending entity, how the addresses were collected,
-- the notice those customers saw, the period it was given in, and which
-- marketing purposes advertise goods like the ones they bought. A customer
-- whose won deal with that entity falls in the period is covered.
SET LOCAL lock_timeout = '3s';

CREATE TABLE existing_customer_notice (
    id uuid DEFAULT uuidv7() NOT NULL,
    legal_entity_id uuid NOT NULL,
    customer_group text NOT NULL,
    collection_channel text NOT NULL,
    notice_text text NOT NULL,
    -- The period the notice was given in; NULL until means it still is.
    effective_from timestamptz NOT NULL,
    effective_until timestamptz,
    -- Where the dated proof of the notice is kept, and the date it bears. A
    -- declaration covers no sale before that date, and without proof none
    -- before the day it was declared.
    evidence_reference text,
    evidence_dated_at timestamptz,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    withdrawn_at timestamptz,
    withdrawn_by text,
    withdrawn_reason text,

    CONSTRAINT existing_customer_notice_pkey PRIMARY KEY (id),
    CONSTRAINT existing_customer_notice_legal_entity_id_fkey FOREIGN KEY (legal_entity_id)
        REFERENCES legal_entity(id) ON DELETE RESTRICT,
    CONSTRAINT existing_customer_notice_group_bounded
        CHECK (length(btrim(customer_group)) BETWEEN 1 AND 500),
    CONSTRAINT existing_customer_notice_channel_bounded
        CHECK (length(btrim(collection_channel)) BETWEEN 1 AND 200),
    CONSTRAINT existing_customer_notice_text_bounded
        CHECK (length(btrim(notice_text)) BETWEEN 1 AND 4000),
    CONSTRAINT existing_customer_notice_period_ordered
        CHECK (effective_until IS NULL OR effective_until > effective_from),
    CONSTRAINT existing_customer_notice_evidence_shape
        CHECK ((evidence_reference IS NULL) = (evidence_dated_at IS NULL)),
    CONSTRAINT existing_customer_notice_evidence_bounded
        CHECK (evidence_reference IS NULL OR length(btrim(evidence_reference)) BETWEEN 1 AND 500),
    CONSTRAINT existing_customer_notice_withdrawal_shape
        CHECK ((withdrawn_at IS NULL) = (withdrawn_by IS NULL)
           AND (withdrawn_at IS NULL) = (withdrawn_reason IS NULL)),
    CONSTRAINT existing_customer_notice_reason_bounded
        CHECK (withdrawn_reason IS NULL OR length(btrim(withdrawn_reason)) BETWEEN 1 AND 500)
);

CREATE INDEX idx_existing_customer_notice_live_entity
    ON existing_customer_notice (legal_entity_id) WHERE withdrawn_at IS NULL;

-- The marketing purposes a declaration covers: the ones that advertise goods
-- or services similar to what its customers bought.
CREATE TABLE existing_customer_notice_purpose (
    notice_id uuid NOT NULL,
    purpose_id uuid NOT NULL,

    CONSTRAINT existing_customer_notice_purpose_pkey PRIMARY KEY (notice_id, purpose_id),
    CONSTRAINT existing_customer_notice_purpose_notice_id_fkey FOREIGN KEY (notice_id)
        REFERENCES existing_customer_notice(id) ON DELETE CASCADE,
    CONSTRAINT existing_customer_notice_purpose_purpose_id_fkey FOREIGN KEY (purpose_id)
        REFERENCES consent_purpose(id) ON DELETE RESTRICT
);

CREATE INDEX idx_existing_customer_notice_purpose_purpose
    ON existing_customer_notice_purpose (purpose_id);

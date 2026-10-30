-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A lost deal names why from a list the administrator keeps, beside its free
-- note. The note stays: it explains the one deal, and the code is what a report
-- can count.
--
-- A deal lost before this file has only its note. An administrator maps each
-- distinct note to a code; the mapping is keyed by a hash of the note, so no
-- copy of the text leaves the deal. A mapped deal says so in
-- lost_reason_basis, which keeps a reviewed guess apart from what the closer
-- chose.
SET LOCAL lock_timeout = '3s';

CREATE TABLE deal_loss_reason (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    key               text NOT NULL UNIQUE,
    label             text NOT NULL,
    sort_order        integer NOT NULL DEFAULT 0,
    active            boolean NOT NULL DEFAULT true,
    system            boolean NOT NULL DEFAULT false,
    shipped_label     text,
    shipped_label_key text,
    label_key         text GENERATED ALWAYS AS (label_key_while_shipped(label, shipped_label, shipped_label_key)) STORED,
    version           bigint NOT NULL DEFAULT 1,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deal_loss_reason_key_shape CHECK (key ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT deal_loss_reason_label_present CHECK (length(btrim(label)) > 0),
    CONSTRAINT deal_loss_reason_version_js_safe
        CHECK (version BETWEEN -9007199254740991 AND 9007199254740991)
);

INSERT INTO deal_loss_reason (key, label, sort_order, system, shipped_label, shipped_label_key) VALUES
    ('price',       'Price',                10, true, 'Price',                'lossReasons.builtin.price'),
    ('competitor',  'Chose a competitor',   20, true, 'Chose a competitor',   'lossReasons.builtin.competitor'),
    ('no_budget',   'No budget',            30, true, 'No budget',            'lossReasons.builtin.no_budget'),
    ('no_decision', 'No decision',          40, true, 'No decision',          'lossReasons.builtin.no_decision'),
    ('timing',      'Bad timing',           50, true, 'Bad timing',           'lossReasons.builtin.timing'),
    ('product_fit', 'Product did not fit',  60, true, 'Product did not fit',  'lossReasons.builtin.product_fit'),
    ('went_silent', 'Stopped responding',   70, true, 'Stopped responding',   'lossReasons.builtin.went_silent'),
    ('other',       'Other',                80, true, 'Other',                'lossReasons.builtin.other');

CREATE TRIGGER trg_deal_loss_reason_updated
    BEFORE UPDATE ON deal_loss_reason
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

-- One reviewed decision per distinct legacy note: the sha256 of the note,
-- trimmed and lower-cased, and the code an administrator chose for it.
CREATE TABLE deal_loss_reason_mapping (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    text_hash   bytea NOT NULL UNIQUE,
    reason_key  text NOT NULL,
    decided_by  text NOT NULL,
    version     bigint NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deal_loss_reason_mapping_hash_shape CHECK (length(text_hash) = 32),
    CONSTRAINT deal_loss_reason_mapping_reason_fkey
        FOREIGN KEY (reason_key) REFERENCES deal_loss_reason (key)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT deal_loss_reason_mapping_version_js_safe
        CHECK (version BETWEEN -9007199254740991 AND 9007199254740991)
);

CREATE INDEX idx_deal_loss_reason_mapping_reason ON deal_loss_reason_mapping (reason_key);

CREATE TRIGGER trg_deal_loss_reason_mapping_updated
    BEFORE UPDATE ON deal_loss_reason_mapping
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

ALTER TABLE deal
    ADD COLUMN lost_reason_code       text,
    ADD COLUMN lost_reason_basis      text,
    ADD COLUMN lost_reason_mapping_id uuid;

-- Added NOT VALID and validated in the next file, so the scan does not hold
-- every deal write. The new columns are empty, so no row can fail.
ALTER TABLE deal
    ADD CONSTRAINT deal_lost_reason_code_fkey
        FOREIGN KEY (lost_reason_code) REFERENCES deal_loss_reason (key)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT deal_lost_reason_mapping_fkey
        FOREIGN KEY (lost_reason_mapping_id) REFERENCES deal_loss_reason_mapping (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT deal_lost_reason_code_only_when_lost
        CHECK (lost_reason_code IS NULL OR status = 'lost') NOT VALID,
    ADD CONSTRAINT deal_lost_reason_basis_check
        CHECK ((lost_reason_code IS NULL) = (lost_reason_basis IS NULL)
            AND (lost_reason_basis IS NULL OR lost_reason_basis IN ('recorded', 'mapped'))
            AND (lost_reason_mapping_id IS NOT NULL) = (lost_reason_basis IS NOT DISTINCT FROM 'mapped')) NOT VALID;

-- A loss now needs a code or a note. A client that sends only the note keeps
-- working, and a closer who picks a code need not also write one. Every row
-- passes: the old rule wanted the note alone.
ALTER TABLE deal DROP CONSTRAINT deal_lost_reason;
ALTER TABLE deal ADD CONSTRAINT deal_lost_reason
    CHECK (status <> 'lost' OR lost_reason IS NOT NULL OR lost_reason_code IS NOT NULL) NOT VALID;

CREATE INDEX idx_deal_lost_reason_code ON deal (lost_reason_code)
    WHERE lost_reason_code IS NOT NULL;
CREATE INDEX idx_deal_lost_reason_mapping ON deal (lost_reason_mapping_id)
    WHERE lost_reason_mapping_id IS NOT NULL;

-- The loss keeps its code in the history row of the move, so a reopen and a
-- second loss leave both answers behind.
ALTER TABLE deal_stage_history
    ADD COLUMN lost_reason_code_at_change text;
ALTER TABLE deal_stage_history
    ADD CONSTRAINT deal_stage_history_lost_reason_code_fkey
        FOREIGN KEY (lost_reason_code_at_change) REFERENCES deal_loss_reason (key)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;
CREATE INDEX idx_deal_stage_history_lost_reason_code ON deal_stage_history (lost_reason_code_at_change)
    WHERE lost_reason_code_at_change IS NOT NULL;

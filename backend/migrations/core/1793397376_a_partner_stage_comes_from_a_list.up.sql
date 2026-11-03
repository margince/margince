-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A partner's stage comes from a list each installation keeps, and every stage
-- names one of six fixed meanings. Code reads the meaning, never the key, so
-- an installation may rename, reorder and add stages freely.
--
-- A partner now holds several kinds. partner_role held one, so each row keeps
-- its role as its first kind and the column goes.
--
-- A partner stands on a stage meaning `active` only once it was activated. The
-- partners already standing on an active or dormant stage were active before
-- the rule existed, so they carry that basis. Nobody recorded when, so their
-- activated_at stays empty rather than holding a guessed date.
SET LOCAL lock_timeout = '3s';

CREATE TABLE partner_stage (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    key               text NOT NULL UNIQUE,
    label             text NOT NULL,
    meaning           text NOT NULL,
    sort_order        integer NOT NULL DEFAULT 0,
    active            boolean NOT NULL DEFAULT true,
    system            boolean NOT NULL DEFAULT false,
    shipped_label     text,
    shipped_label_key text,
    label_key         text GENERATED ALWAYS AS (label_key_while_shipped(label, shipped_label, shipped_label_key)) STORED,
    version           bigint NOT NULL DEFAULT 1,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT partner_stage_key_shape CHECK (key ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT partner_stage_label_present CHECK (length(btrim(label)) > 0),
    CONSTRAINT partner_stage_meaning_check
        CHECK (meaning IN ('candidate', 'signed', 'ready', 'active', 'dormant', 'ended')),
    CONSTRAINT partner_stage_version_js_safe
        CHECK (version BETWEEN -9007199254740991 AND 9007199254740991)
);

INSERT INTO partner_stage (key, label, meaning, sort_order, system, shipped_label, shipped_label_key) VALUES
    ('research',          'Research',          'candidate',  10, true, 'Research',          'partnerStages.builtin.research'),
    ('identified',        'Identified',        'candidate',  20, true, 'Identified',        'partnerStages.builtin.identified'),
    ('contacted',         'Contacted',         'candidate',  30, true, 'Contacted',         'partnerStages.builtin.contacted'),
    ('in_conversation',   'In conversation',   'candidate',  40, true, 'In conversation',   'partnerStages.builtin.in_conversation'),
    ('fit_confirmed',     'Fit confirmed',     'candidate',  50, true, 'Fit confirmed',     'partnerStages.builtin.fit_confirmed'),
    ('agreement_pending', 'Contract pending',  'candidate',  60, true, 'Contract pending',  'partnerStages.builtin.agreement_pending'),
    ('contract_signed',   'Contract signed',   'signed',     70, true, 'Contract signed',   'partnerStages.builtin.contract_signed'),
    ('ready',             'Ready',             'ready',      80, true, 'Ready',             'partnerStages.builtin.ready'),
    ('activated',         'Activated',         'active',     90, true, 'Activated',         'partnerStages.builtin.activated'),
    ('active',            'Active',            'active',    100, true, 'Active',            'partnerStages.builtin.active'),
    ('active_referring',  'Active, referring', 'active',    110, true, 'Active, referring', 'partnerStages.builtin.active_referring'),
    ('dormant',           'Dormant',           'dormant',   120, true, 'Dormant',           'partnerStages.builtin.dormant'),
    ('no_fit',            'No fit',            'ended',     130, true, 'No fit',            'partnerStages.builtin.no_fit');

CREATE TRIGGER trg_partner_stage_updated
    BEFORE UPDATE ON partner_stage
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

ALTER TABLE partner DROP CONSTRAINT partner_relationship_stage_check;
ALTER TABLE partner ALTER COLUMN relationship_stage DROP DEFAULT;

ALTER TABLE partner
    ADD COLUMN partner_kinds      text[] NOT NULL DEFAULT '{}',
    ADD COLUMN activated_at       timestamptz,
    ADD COLUMN activation_basis   text,
    ADD COLUMN activated_by       text,
    ADD COLUMN activation_lead_id uuid,
    ADD COLUMN activation_deal_id uuid;

-- The partner table holds one row per partner company, so the rewrite is
-- small. The version trigger stays off for it: no human changed these rows.
ALTER TABLE partner DISABLE TRIGGER trg_partner_updated;
UPDATE partner
   SET partner_kinds = CASE WHEN partner_role IS NULL THEN '{}'::text[] ELSE ARRAY[partner_role] END,
       activation_basis = CASE WHEN relationship_stage IN ('active', 'active_referring', 'dormant')
                               THEN 'recorded_before_ladder' END
 WHERE partner_role IS NOT NULL
    OR relationship_stage IN ('active', 'active_referring', 'dormant');
ALTER TABLE partner ENABLE TRIGGER trg_partner_updated;

ALTER TABLE partner DROP COLUMN partner_role;

-- Added NOT VALID and validated in the next file, so the scan does not hold
-- every partner write.
ALTER TABLE partner
    ADD CONSTRAINT partner_relationship_stage_fkey
        FOREIGN KEY (relationship_stage) REFERENCES partner_stage (key)
        ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT partner_partner_kinds_check
        CHECK (partner_kinds <@ ARRAY['hosting', 'consulting', 'strategic', 'referral']::text[]) NOT VALID,
    ADD CONSTRAINT partner_activation_basis_check
        CHECK (activation_basis IS NULL
            OR activation_basis IN ('lead_accepted', 'customer_won', 'recorded_before_ladder')) NOT VALID,
    ADD CONSTRAINT partner_activation_pairing
        CHECK ((activation_basis IS NULL AND activated_at IS NULL AND activated_by IS NULL
                AND activation_lead_id IS NULL AND activation_deal_id IS NULL)
            OR (activation_basis = 'recorded_before_ladder' AND activated_at IS NULL
                AND activation_lead_id IS NULL AND activation_deal_id IS NULL)
            OR (activation_basis = 'lead_accepted' AND activated_at IS NOT NULL AND activation_lead_id IS NOT NULL)
            OR (activation_basis = 'customer_won' AND activated_at IS NOT NULL AND activation_deal_id IS NOT NULL)) NOT VALID;

-- A full index, not the partial one it replaces: deleting a stage asks whether
-- any partner, archived ones included, still carries it.
DROP INDEX idx_partner_stage;
CREATE INDEX idx_partner_stage ON partner (relationship_stage);
CREATE INDEX idx_partner_kinds ON partner USING gin (partner_kinds);

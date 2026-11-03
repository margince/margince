-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A partner on a stage the old list does not name, or holding only the
-- referral kind, cannot be told apart from what the old columns could hold. The
-- stage falls back to research and the role to null; the audit log keeps what
-- each row held.
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_partner_kinds;
DROP INDEX idx_partner_stage;
CREATE INDEX idx_partner_stage ON partner (relationship_stage) WHERE archived_at IS NULL;

ALTER TABLE partner
    DROP CONSTRAINT partner_activation_pairing,
    DROP CONSTRAINT partner_activation_basis_check,
    DROP CONSTRAINT partner_partner_kinds_check,
    DROP CONSTRAINT partner_relationship_stage_fkey;

ALTER TABLE partner ADD COLUMN partner_role text;

ALTER TABLE partner DISABLE TRIGGER trg_partner_updated;
UPDATE partner
   SET partner_role = (SELECT k FROM unnest(partner_kinds) AS k
                        WHERE k IN ('hosting', 'consulting', 'strategic')
                        ORDER BY k LIMIT 1),
       relationship_stage = CASE
           WHEN relationship_stage IN ('research', 'identified', 'contacted', 'in_conversation', 'fit_confirmed',
                                       'agreement_pending', 'active', 'active_referring', 'dormant', 'no_fit')
           THEN relationship_stage ELSE 'research' END;
ALTER TABLE partner ENABLE TRIGGER trg_partner_updated;

ALTER TABLE partner
    DROP COLUMN activation_deal_id,
    DROP COLUMN activation_lead_id,
    DROP COLUMN activated_by,
    DROP COLUMN activation_basis,
    DROP COLUMN activated_at,
    DROP COLUMN partner_kinds;

ALTER TABLE partner ALTER COLUMN relationship_stage SET DEFAULT 'research';
ALTER TABLE partner
    ADD CONSTRAINT partner_partner_role_check
        CHECK (partner_role IS NULL OR partner_role IN ('hosting', 'consulting', 'strategic')) NOT VALID,
    ADD CONSTRAINT partner_relationship_stage_check
        CHECK (relationship_stage IN ('research', 'identified', 'contacted', 'in_conversation', 'fit_confirmed',
                                      'agreement_pending', 'active', 'active_referring', 'dormant', 'no_fit')) NOT VALID;
-- Left NOT VALID: the rows were just rewritten to pass, and a rollback should
-- not wait on a scan.

DROP TABLE partner_stage;

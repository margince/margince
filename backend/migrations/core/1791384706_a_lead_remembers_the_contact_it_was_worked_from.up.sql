SET LOCAL lock_timeout = '3s';
-- A lead created from an existing contact records which one. Dedupe keys on
-- email and LinkedIn profile, so a contact with neither could become two live
-- leads; the partial unique index holds one live lead per contact, and a
-- contact that is deleted leaves its lead standing without the link.
ALTER TABLE lead ADD COLUMN from_contact_id uuid REFERENCES contact(id) ON DELETE SET NULL;
CREATE UNIQUE INDEX uq_lead_from_contact_live ON lead (from_contact_id)
    WHERE from_contact_id IS NOT NULL AND archived_at IS NULL;

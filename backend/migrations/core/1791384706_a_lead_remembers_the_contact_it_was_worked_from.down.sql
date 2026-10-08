SET LOCAL lock_timeout = '3s';
DROP INDEX idx_lead_from_contact;
DROP INDEX uq_lead_from_contact_live;
ALTER TABLE lead DROP COLUMN from_contact_id;

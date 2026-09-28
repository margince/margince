SET LOCAL lock_timeout = '5s';

ALTER TABLE lead
    ALTER COLUMN score TYPE integer,
    ALTER COLUMN score_computed TYPE integer;

-- Restored as it stood: UNIQUE (id), redundant with lead_pkey.
ALTER TABLE lead ADD CONSTRAINT uq_lead_ws_id UNIQUE (id);

-- A lead linked to no company says its industry itself. A linked company's
-- industry speaks for the lead on read; this value stays stored beside it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead ADD COLUMN industry text;

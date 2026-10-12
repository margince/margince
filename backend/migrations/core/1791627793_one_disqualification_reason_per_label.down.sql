-- Reverses 1791627793. A twin the up step renamed keeps its new label.
SET LOCAL lock_timeout = '3s';
DROP INDEX IF EXISTS lead_disqualify_reason_label_once;

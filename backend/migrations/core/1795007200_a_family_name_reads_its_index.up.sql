-- The candidate read finds an employer's colleagues by family name, so the
-- employer arm reads the few contacts that share it and not the whole staff.
-- The expression is familyNameKeySQL in the contacts module, character for
-- character, or the planner cannot use the index.
SET LOCAL lock_timeout = '3s';
CREATE INDEX IF NOT EXISTS idx_contact_family_name_key
  ON contact (substring(btrim(regexp_replace(replace(f_fold_apostrophes(lower(CASE WHEN full_name LIKE '%,%' THEN split_part(full_name, ',', 1) ELSE full_name END)), 'ς', 'σ'), '\s+', ' ', 'g')) from '[^ ]+$'))
  WHERE archived_at IS NULL;

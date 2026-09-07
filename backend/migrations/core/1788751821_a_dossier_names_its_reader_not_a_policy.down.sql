-- 1788751821 (down): a_dossier_names_its_reader_not_a_policy.
--
-- Deliberately NOT the sentence this replaced. Restoring it would put the false
-- claim back into the catalog, and a down migration that reintroduces a defect
-- is not a rollback. The comment returns to naming the column and nothing else.
COMMENT ON COLUMN org_dossier.user_id IS
    'The reader this assembly was generated for.';

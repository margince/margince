-- 1788751821: a_dossier_names_its_reader_not_a_policy.
--
-- The column comment credits row-level security with bounding the workspace.
-- Core has no row-level security: no table carries a workspace column and no policy
-- reads one at all. An operator running \d+ org_dossier is told a control exists that
-- does not, and the comment is the only place they would look.
--
-- A new migration rather than an edit to the baseline: 0001 is applied, and an
-- applied version never re-runs, so editing it would change what a FRESH
-- installation is told while every deployed catalog kept the old sentence.
COMMENT ON COLUMN org_dossier.user_id IS
    'The reader this assembly was generated for. Written into every read explicitly: the reader key is what keeps two readers'' assemblies apart, and the installation is the one workspace (ADR-0061).';

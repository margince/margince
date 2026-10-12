-- project.visibility could only ever be 'workspace': that was its default, and
-- a CHECK admitted no other value. Nothing wrote it and nothing read it — the
-- row visibility projects actually have is auth.ScopeClauseFor's, computed from
-- the caller's scope rather than stored.
--
-- A column that cannot carry information costs a byte a row and, far more, reads
-- to the next contributor as a dimension the product has: they group a report by
-- it, or draw a control that switches on it, and find one bucket.
SET LOCAL lock_timeout = '3s';

ALTER TABLE project DROP CONSTRAINT project_visibility_check;
ALTER TABLE project DROP COLUMN visibility;

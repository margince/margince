-- A stored mail cause is bounded wherever one is stored, and `notice` was the
-- half that was not.
--
-- The truncation on the way in is ONE constant over both columns
-- (notices.maxMailErrorRunes), so a bound on only one of them makes that
-- constant a convention rather than a rule — and a driver's error text is
-- unbounded, so what the column would hold is a row the product cannot render.
--
-- NOT VALID, and the VALIDATE is a migration of its own rather than the next
-- line here. A plain ADD CONSTRAINT scans every existing row while holding
-- ACCESS EXCLUSIVE, and the runner wraps a whole file in ONE transaction, so a
-- VALIDATE below would run under that same lock — on a notice table with a row
-- per thing the product has ever told anybody, that scan is the outage.
SET LOCAL lock_timeout = '3s';

ALTER TABLE notice
  ADD CONSTRAINT notice_email_error_bounded
    CHECK (length(email_error) <= 500) NOT VALID;

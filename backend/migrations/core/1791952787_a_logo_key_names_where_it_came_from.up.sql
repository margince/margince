-- A stored logo and its provenance arrive together, or neither does.
--
-- Nothing stopped a row holding logo_object_key with no logo_origin. The read
-- then serves an image whose origin is unknown, and the one question a reader
-- asks about a logo somebody disputes is where it came from.
--
-- Two constraints rather than one chained CHECK, unlike
-- activity_reply_verdict_stamped: a refusal then names the pair that is wrong
-- rather than the row, and the icon is written by a different path from the
-- full logo.
--
-- NOT VALID because company is a written table. The rows are validated in the
-- migration beside this one, so the scan does not hold a write lock here.
-- Bounded, because ADD CONSTRAINT takes a lock on a written table even with
-- the scan deferred: a queue behind it is worse than a migration that retries.
SET LOCAL lock_timeout = '5s';

ALTER TABLE company
    ADD CONSTRAINT company_logo_names_its_origin
    CHECK ((logo_object_key IS NULL) = (logo_origin IS NULL)) NOT VALID;

ALTER TABLE company
    ADD CONSTRAINT company_logo_icon_names_its_origin
    CHECK ((logo_icon_object_key IS NULL) = (logo_icon_origin IS NULL)) NOT VALID;

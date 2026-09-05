-- Which claim of a website read is current, and when it became current — the
-- two facts the AI-activity projection needs from a carrier and only the
-- carrier can hold. attachment_extraction gained the same pair for the same
-- reason; the reasoning is theirs and is not restated here.
--
-- The dossier's lifecycle is not monotonic: a running read is deferred when the
-- budget runs out and claimed again when it returns, a retryable failure names
-- its own next attempt and is claimed again past it, and a dead worker's claim
-- is reclaimed in place once its lease lapses. Each of those begins a NEW
-- attempt at the same read, and a projection ordering two events for one
-- occurrence on state alone cannot tell "claimed again" from a stale redelivery
-- of the first claim. The count lives here rather than with the projection
-- because which claim is current is the source's fact.
--
-- attempt_at is what a LIVE occurrence ages from. created_at is that instant for
-- the first attempt only; a read claimed again hours after it was created and
-- dated by created_at is past its lease before the worker has fetched a page.
--
-- ACCESS EXCLUSIVE on a table this migration did not create: instant, no row is
-- rewritten, but the lock queues behind every open transaction on the table.
-- Three seconds, so a migration that cannot get in fails the deploy loudly.
SET LOCAL lock_timeout = '3s';

ALTER TABLE site_read
  ADD COLUMN attempt    integer     NOT NULL DEFAULT 1 CHECK (attempt >= 1),
  ADD COLUMN attempt_at timestamptz NOT NULL DEFAULT now();

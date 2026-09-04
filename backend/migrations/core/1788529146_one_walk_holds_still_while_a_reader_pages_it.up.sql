-- One reader's walk through their day, frozen at the instant it started.
--
-- WHAT WAS WRONG. The queue's cursor is an offset into a ranking rebuilt on
-- every read, and the contract said so plainly: a row crossing the page
-- boundary between two reads is served twice or not at all. That is honest and
-- it is still a bad walk. A rep paging their morning saw the count above the
-- queue move under them, met the same customer twice, and had no way to know
-- which of the two had happened.
--
-- WHAT IS STORED, AND WHAT IS DELIBERATELY NOT. Identity and order only: which
-- rows this walk covers and in which sequence. NO TITLES, no subjects, no
-- message excerpts, no person or company names, no evidence. A snapshot that
-- froze display text would be a second copy of records whose visibility can
-- change underneath it, and re-serving that copy after a revocation would
-- disclose exactly what the revocation withdrew. Every page re-reads the live
-- rows under the caller's own gates and shows only what they may see NOW; the
-- snapshot decides order and membership, never content.
--
-- MEMBERSHIP MOVES IN ONE DIRECTION. New work waits for the reader to refresh,
-- so the walk they started is the walk they finish. Work that was resolved,
-- deleted, or is no longer visible LEAVES immediately — the totals here can
-- fall, and the response says how many rows went. Freezing a headline over work
-- the reader can no longer see or do would be a steadier number and a false
-- one.
--
-- PER READER, and keyed on them: a walk is one person's position in one
-- question. The fingerprint beside it is the question — scope, filter, owner —
-- so a token carried onto a different one is refused rather than resumed into
-- an answer nobody asked for.
--
-- No audit and no event. This is per-reader derived state, the shape
-- org_brief, person_brief and deal_status_card already have: an assembly
-- generated FOR one person and never served to another. An audit trail over it
-- would record reading rather than changing.
--
-- No workspace_id and no row-level security: the unit tables in this schema
-- carry neither, and the workspace binding is the transaction's.
SET LOCAL lock_timeout = '3s';

CREATE TABLE worklist_snapshot (
  id uuid DEFAULT uuidv7() PRIMARY KEY,
  reader_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  -- The question this walk answers, as the cursor fingerprints it. Held so a
  -- snapshot cannot be resumed under a different scope, filter or owner.
  params_fingerprint text NOT NULL,
  -- When the day was assembled. Reported to the client so a reader can be told
  -- how old the walk they are still paging is.
  as_of timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  -- A walk is one sitting. Past this the token is refused and the client starts
  -- a fresh snapshot, which is a better answer than resuming a walk through a
  -- day that has since ended.
  expires_at timestamptz NOT NULL,
  -- The frozen partition: how much work the walk started with, so the headline
  -- does not climb as new work arrives behind the reader.
  buckets jsonb NOT NULL,
  -- The ordered identities: [{"source": "...", "row_id": "..."}]. row_id is
  -- text rather than uuid because a folded group carries a synthetic key its
  -- lane mints, and a column that could only hold a uuid would make those rows
  -- unwalkable for a reason no reader could see.
  rows jsonb NOT NULL,
  -- A walk that ends before it starts is a row that can never be resumed, and
  -- the read side would have to wonder whether it meant anything.
  CONSTRAINT worklist_snapshot_forward CHECK (expires_at > created_at)
);

-- The two reads this table serves: resume THIS walk, and sweep a reader's
-- expired ones when they start a new one. Both begin with the reader.
CREATE INDEX worklist_snapshot_by_reader
    ON worklist_snapshot (reader_id, created_at DESC);

COMMENT ON TABLE worklist_snapshot IS
    'One reader''s walk through their worklist, frozen at the instant it started. Identity and order only — never display text, which every page re-reads live under the caller''s own gates.';

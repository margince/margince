-- What each mailbox asks of the mail it brings in, whose counterparties one
-- seat holds back, and the ledger that decides when a thread stops being held.
--
-- Until now capture had one workspace-wide answer (capture.mail_sharing) and no
-- per-mailbox one. A founder and a sales rep connecting the same workspace got
-- the same treatment, which is wrong in both directions: the rep's customer
-- correspondence is the point of a shared CRM, and the founder's mail to their
-- lawyer is nobody else's.

-- mail_posture is the per-mailbox answer, and it defaults to 'classified': a
-- message is held to its participants until something judges it ordinary.
--
-- The default is the whole safety property. An older binary, a missed insert
-- path or a connection row created by a future caller that forgets this column
-- all produce a HELD mailbox, never a shared one. A default of 'shared' would
-- fail the other way, and the failure would be silent — mail readable by
-- colleagues that nobody decided to share.
--
-- ADD COLUMN ... DEFAULT backfills every existing row, so a mailbox connected
-- the day before this migration lands on the same answer as one connected the
-- day after. That is deliberate rather than incidental: born held is the
-- product's position, and neither mailbox is classified by somebody's decision.
--
-- The cost is real and worth stating. Mail already captured keeps the audience
-- it has — this moves the POSTURE, not history — but every message captured
-- from here on is held to its participants until a classifier judges its
-- thread, and until that classifier ships there is nothing to open one. An
-- installation that wants the old behaviour turns on
-- capture.shared_posture_allowed and sets the mailbox to 'shared'.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection
    ADD COLUMN mail_posture text NOT NULL DEFAULT 'classified';

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_mail_posture_check
    CHECK (mail_posture IN ('shared', 'classified', 'held'));

COMMENT ON COLUMN capture_connection.mail_posture IS
    'What this mailbox asks of the mail it brings in: shared (colleagues read it), classified (held until a verdict opens it), held (no verdict opens it). Default classified; a rebind resets to held.';

-- One seat's decision that a counterparty's mail is nobody else's.
--
-- Per user, never per workspace: one seat's lawyer says nothing about another
-- seat's, and a workspace-wide list would let anyone hold a colleague's
-- customer out of the shared CRM by naming their domain.
CREATE TABLE capture_counterparty_hold (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE RESTRICT,
    kind text NOT NULL,
    value text NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capture_counterparty_hold_kind_check CHECK (kind IN ('address', 'domain')),
    -- Folded, like every other address this module stores, so a lookup never
    -- has to guess which case the writer used.
    CONSTRAINT capture_counterparty_hold_value_check CHECK ((value = lower(value)) AND (value <> '')),
    CONSTRAINT capture_counterparty_hold_user_value_key UNIQUE (user_id, kind, value)
);

-- What a classifier concluded about one thread, for one seat.
--
-- Per thread AND per seat, because a thread reaching two mailboxes is two
-- seats' correspondence and each may conclude differently about it — the same
-- reason capture_import exists. The audience derivation takes the strictest.
CREATE TABLE capture_thread_verdict (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    thread_key text NOT NULL,
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE RESTRICT,
    -- The message that opened the thread for this seat, so a verdict can be
    -- traced to what it was asked about. Nullable: the activity may be erased
    -- while the verdict stands, and losing the verdict would re-open a thread
    -- a classifier already held.
    first_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL,
    status text NOT NULL DEFAULT 'pending',
    kind text,
    confidence numeric,
    attempts integer NOT NULL DEFAULT 0,
    -- The claim triple, the shape capture_pending_counterparty already uses: a
    -- worker takes a row for a bounded window, and an abandoned claim expires
    -- rather than wedging the row forever.
    claimed_by text,
    claimed_until timestamptz,
    next_attempt_at timestamptz,
    disposition_reason text,
    -- The exact addresses the verdict SAW. A later message on this thread
    -- inherits a cleared verdict only when its sender is one of them: a thread
    -- cleared with the customer on it says nothing about a message from their
    -- lawyer, and inheriting by DOMAIN would let one new address on a settled
    -- thread carry whatever it wants into a shared timeline.
    seen_addresses text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    CONSTRAINT capture_thread_verdict_status_check CHECK (status IN
        ('pending', 'cleared', 'held', 'unsure', 'shared_by_owner', 'held_by_owner')),
    CONSTRAINT capture_thread_verdict_thread_user_key UNIQUE (thread_key, user_id)
);

-- The due-work scan: every pending row whose next attempt has come round.
-- Partial, because a resolved verdict is never scanned again and the table
-- keeps one row per thread per seat for the life of the thread.
CREATE INDEX capture_thread_verdict_due_idx
    ON capture_thread_verdict (next_attempt_at)
 WHERE status = 'pending' AND next_attempt_at IS NOT NULL;

-- The per-seat walk: every message one seat imported from a given counterparty,
-- which the hold and its reversal both need.
CREATE INDEX capture_counterparty_hold_user_idx
    ON capture_counterparty_hold (user_id, kind);

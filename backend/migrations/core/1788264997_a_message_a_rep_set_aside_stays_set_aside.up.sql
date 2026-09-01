-- A rep who judges a waiting message can put it down, and it stays down.
--
-- The Worklist's whole claim is that it is finite: work it, and it empties. An
-- unanswered message the rep has already decided about — the newsletter that
-- is not a customer, the reply that is somebody else's, the one they will get
-- to on Thursday — had no way to leave, so it came back every morning and the
-- count above it stayed wrong. A queue that cannot be cleared is a list.
--
-- TWO TABLES, and the split is the point rather than a normalization.
--
-- "This thread is not sales" is a fact about the THREAD. One rep deciding the
-- procurement newsletter is not a customer conversation is deciding it for
-- everybody, because it is a statement about what the message is.
--
-- "I have set this aside" and "this is not mine" are facts about one READER.
-- A rep snoozing a reply until Thursday must not remove it from the colleague
-- who owns the deal it is on, and a rep saying it is not theirs is saying
-- exactly that it belongs to somebody else — who must still see it.
--
-- One table keyed on the activity alone would collapse the two, and the
-- collapse fails in the direction nobody notices: the row simply stops
-- appearing for a person who never judged it, and no screen anywhere says a
-- colleague hid it.
SET LOCAL lock_timeout = '3s';

-- The thread's own judgement.
--
-- Keyed on the THREAD, not on one message in it. A judgement keyed on an
-- activity id is undone by the next reply: the newer inbound is a different
-- row, becomes the thread's waiting candidate, and the newsletter the rep
-- already dismissed is back on every queue — which is the opposite of what
-- "this thread is not sales" means.
--
-- The key is the triple the waiting query itself matches a thread by: a mail
-- thread key comes from headers the sender controls and channel keys share the
-- flat namespace with them, so key alone would let a crafted References value
-- silence an unrelated conversation.
--
-- channel_provider is NULLABLE on activity, and NULL is a real value here (a
-- mail thread has no provider). The primary key therefore uses a sentinel
-- rather than the column: two NULLs never conflict in a PK, so every mail
-- thread judged not-sales would insert a second row instead of updating the
-- first, and un-judging one would leave the others hiding it.
CREATE TABLE activity_sales_state (
  thread_key text NOT NULL,
  kind text NOT NULL,
  channel_provider text NOT NULL DEFAULT '',
  set_by text NOT NULL,
  set_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (thread_key, kind, channel_provider)
);

-- One reader's own judgement, per message.
CREATE TABLE activity_reader_state (
  activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
  reader_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  -- snoozed carries a moment it stops applying; not_mine does not, because
  -- "this is not my work" does not expire on a date.
  state text NOT NULL,
  snoozed_until timestamptz,
  set_by text NOT NULL,
  set_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (activity_id, reader_id),
  CONSTRAINT activity_reader_state_known CHECK (state IN ('snoozed', 'not_mine')),
  -- A snooze with no moment would never lift, which is not a snooze; a
  -- not_mine with one would lift, which is not a hand-off. The two shapes are
  -- held here so neither can be written by halves.
  CONSTRAINT activity_reader_state_shaped CHECK (
    (state = 'snoozed' AND snoozed_until IS NOT NULL)
    OR (state = 'not_mine' AND snoozed_until IS NULL)
  )
);

-- The read this exists for: "which of these messages has THIS reader put
-- down", asked once per Worklist assembly against the reader's own rows.
CREATE INDEX activity_reader_state_by_reader ON activity_reader_state (reader_id, activity_id);

-- The occurrence carries what it was ABOUT, named.
--
-- ai_task_run already held subject_type and subject_id, which say WHICH record
-- an occurrence concerned and nothing about what a reader would call it. The
-- rail could therefore say "I'm reading your document" and never which one,
-- which is a sentence about software rather than about the reader's afternoon.
--
-- The label is stored rather than resolved on read, and that is the whole
-- design: this projection owns one table and reaches back into no source's, so
-- a name it is not handed is a name it cannot have. The label travels on
-- ai_task.state_changed for exactly the reason the lease does — only the source
-- knows it, the projection must not guess.
--
-- Two consequences, both intended. It is a SNAPSHOT: a record renamed later
-- keeps its old name on a settled line, which is what that line was actually
-- about. And it cannot be re-gated at read time, which is why a source emits a
-- label only where the occurrence's own actor is the person that record was
-- already displayed to.
--
-- Additive: existing rows keep NULL, and NULL is the ordinary case rather than
-- a gap — most kinds are about no single record, and a reader with no name is
-- shown the generic sentence that shipped before this column existed.
--
-- Bounded: the ALTER below takes a lock on a table this migration did not
-- create, so an open transaction holding a conflicting one would otherwise
-- stall every ai_task_run write for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_task_run ADD COLUMN subject_label text NULL
  CHECK (subject_label IS NULL OR length(subject_label) <= 120);

COMMENT ON COLUMN ai_task_run.subject_label IS
  'What the subject is called, as the source knew it when it emitted. A snapshot: never re-resolved and never re-gated on read.';

-- A decision names the rules it was judged under.
--
-- messaging.Rules has said since it shipped that the version is "stamped onto
-- every decision taken under it". Nothing stamped it. A subject asking which
-- rules judged their message could be answered only by re-deriving them from
-- whatever the code says today, which is the opposite of a record.
--
-- TWO COLUMNS, because one cannot answer it. A single jurisdiction's rule set
-- carries its own version; a fold of two carries none, because
-- messagingrules.stricter zeroes it rather than misnaming the fold with one
-- country's number. ruleset_codes answers in both cases, and ruleset_version is
-- meaningful exactly when one code applied.
--
-- NULL on both is a real answer and not a gap: an installation that declares no
-- country resolves to no rules at all, and the consent requirement that binds
-- everywhere is enforced by the marketing verdict, which runs before any pack.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision
  ADD COLUMN ruleset_version int,
  ADD COLUMN ruleset_codes text[];

-- A version without the codes that produced it names nothing, and codes with a
-- version when more than one applied would claim a fold had a single version.
--
-- coalesce, because array_length answers NULL for an EMPTY array rather than 0,
-- and a CHECK evaluating to NULL is accepted. Written as `= 1` alone, the
-- constraint would have admitted a version alongside zero jurisdictions — the
-- exact pair it exists to refuse.
--
-- An empty array is also refused outright: a decision that recorded no codes
-- must say so with NULL, so "no jurisdiction applied" is one answer in the
-- record instead of two that read the same.
ALTER TABLE communication_decision
  ADD CONSTRAINT communication_decision_ruleset_shape CHECK (
    (ruleset_codes IS NULL OR coalesce(array_length(ruleset_codes, 1), 0) > 0)
    AND (ruleset_version IS NULL OR coalesce(array_length(ruleset_codes, 1), 0) = 1)
  );

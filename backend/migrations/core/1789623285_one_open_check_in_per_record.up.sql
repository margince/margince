-- One open check-in per record, for the reminders written before the draw
-- learned to hold.
--
-- The quiet-account starters used to ask once per record and once per anchor: a
-- silent account produced a reminder on the account, one on each open deal, and
-- one on each stakeholder contact, with a fresh set every time the anchor moved.
-- One staging day wrote 72. The scan now collapses those onto the account and
-- skips a record whose reminder is still unanswered, but it recognises a
-- reminder by its source_system, and the tasks already written carry none. They
-- are invisible to the new hold, so without this repair the first pass after
-- deploy adds one more beside each of them.
--
-- Archival rather than completion, as 1787873299 argued for the same shape: a
-- migration has no actor who could honestly complete somebody's task, while
-- archival is the visibility retirement any operator could perform by hand. It
-- is also why this file writes no audit or outbox row — there is no principal to
-- attribute one to, and inventing "system" for a repair nobody ran would put a
-- name on the record that answers for nothing. The rows it retires are the
-- product's own automation output, never a person's writing.

SET LOCAL lock_timeout = '3s';

-- The bound, spelled once and shared by every statement below: an OPEN task the
-- automation engine wrote, carrying one of the two quiet-account subjects.
--
-- captured_by rides the source because source alone is client-writable, and the
-- pattern arm matters as much as the equality: the time scanner binds its own
-- namespaced principal, so 'system' alone would miss every row this repair is
-- about. 1787873299 used equality only, which is why it never saw these.
--
-- The subject prefix is what tells the two starters apart in rows that carry no
-- source_system yet. It is the only signal left: the wording is the handler's
-- own, fixed in handlers_clock.go.
CREATE TEMPORARY TABLE legacy_reminder ON COMMIT DROP AS
SELECT a.id,
       CASE WHEN a.subject LIKE 'Check in — no activity since %'
            THEN 'no_activity_reminder' ELSE 'check_in_cadence' END AS handler,
       a.created_at,
       l.entity_type,
       coalesce(l.contact_id, l.company_id, l.deal_id, l.lead_id, l.project_id) AS entity_id,
       coalesce(d.company_id, e.company_id) AS absorbing_company
FROM activity a
JOIN activity_link l ON l.activity_id = a.id
LEFT JOIN deal d ON d.id = l.deal_id
LEFT JOIN relationship e ON e.contact_id = l.contact_id
                        AND e.kind = 'employment'
                        AND e.ended_at IS NULL
                        AND e.archived_at IS NULL
WHERE a.kind = 'task'
  AND a.is_done = false
  AND a.archived_at IS NULL
  AND a.source = 'system'
  AND (a.captured_by = 'system' OR a.captured_by LIKE 'system:%')
  AND a.source_system IS NULL
  AND (a.subject LIKE 'Check in — no activity since %'
    OR a.subject LIKE 'Time for a check-in — last touched %');

-- 1. Duplicates on ONE record fold to the newest.
--
-- Partitioned by handler as well as by record: the two starters ask different
-- questions on the same cadence, and handlers_clock_test.go holds their
-- identities apart deliberately. Folding them together would retire a question
-- nobody has answered.
--
-- The NEWEST survives, where 1787873299 kept the earliest. The copies here
-- differ by anchor rather than by accident — each was minted when the anchor
-- moved — so the latest is the one naming the date a rep would recognise.
-- created_at breaks ties with id beneath it, so the choice is deterministic on
-- rows written inside one transaction.
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY handler, entity_type, entity_id
               ORDER BY created_at DESC, id DESC) AS copy_rank
    FROM legacy_reminder
)
UPDATE activity SET archived_at = now()
WHERE id IN (SELECT id FROM ranked WHERE copy_rank > 1);

-- 2. A child's reminder folds into its account's.
--
-- The runtime collapses a deal, and a contact currently employed by the account,
-- onto the account itself. A legacy pair that straddles that rule — one reminder
-- on the deal, one on its company — is two questions about one silence, and the
-- account's is the one the scan would write today.
--
-- Only where the account ALREADY holds its own open reminder. A child reminder
-- whose account has none is the only question anyone is being asked about that
-- silence, and archiving it would leave the account unwatched until its next
-- quiet spell — the same starvation the collapse itself had to avoid.
UPDATE activity SET archived_at = now()
WHERE id IN (
    SELECT child.id
    FROM legacy_reminder child
    JOIN legacy_reminder parent
      ON parent.entity_type = 'company'
     AND parent.entity_id = child.absorbing_company
     AND parent.handler = child.handler
    JOIN activity pa ON pa.id = parent.id AND pa.archived_at IS NULL
    WHERE child.entity_type IN ('deal', 'contact')
      AND child.absorbing_company IS NOT NULL);

-- 3. What survives takes an identity the draw can see.
--
-- Bounded to rows whose provenance is NULL, which legacy_reminder already is, so
-- a re-run rewrites nothing: the second pass finds no unstamped row and updates
-- zero rows rather than bumping updated_at and version on every survivor.
--
-- The id makes each key unique, which uq_activity_source (source_system,
-- source_id) requires. 'legacy:' keeps them out of the shape the handlers mint
-- today — handler:entity:anchor — so a stamped row can never collide with a
-- reminder the engine writes later for the same occurrence.
UPDATE activity a
SET source_system = r.handler,
    source_id = 'legacy:' || a.id::text
FROM legacy_reminder r
WHERE a.id = r.id
  AND a.archived_at IS NULL
  AND a.source_system IS NULL;

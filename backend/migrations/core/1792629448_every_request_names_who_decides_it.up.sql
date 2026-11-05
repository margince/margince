-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- Every pending request names the one member it is for.
--
-- Before this, who a request was for was worked out on every read, and most
-- kinds answered "nobody". The notifier then told every seat allowed to decide,
-- which put an admin in front of every deal close an agent staged for somebody
-- else. Staging now stores the member (approvals.resolveDecider), and a pending
-- row without one is refused by the CHECK below.
--
-- requested_decider_source records which rule named the member, because only a
-- member named as the record's owner follows the record when its owner changes.
-- The constraints are added NOT VALID: the table is old and large, and a plain
-- add would scan it under an exclusive lock. New and updated rows are checked
-- at once; the backfill below makes the existing pending rows comply.
ALTER TABLE approval
    ADD COLUMN requested_decider_id uuid,
    ADD COLUMN requested_decider_source text;

ALTER TABLE approval
    ADD CONSTRAINT approval_requested_decider_fkey
        FOREIGN KEY (requested_decider_id) REFERENCES app_user(id) ON DELETE SET NULL NOT VALID,
    ADD CONSTRAINT approval_requested_decider_source_check
        CHECK (requested_decider_source IN ('payload_owner', 'on_behalf_of', 'stager', 'record_owner')) NOT VALID;

-- Pending rows get the member the staging path would name today, in the same
-- order: the payload's owner, the human the work was for, the human who staged
-- it, the owner of the record it changes. Only an active or invited member is
-- named.
--
-- A promise (commitment_task) is put only to the colleague who made it, or for
-- an existing task to that task's assignee. A private one is named only when
-- its own owner made the promise. No other rule applies to it, so one staged
-- for a reader, a mailbox owner or a deal owner is closed below.
WITH resolved AS (
    SELECT a.id, d.member, d.source
      FROM approval a
      CROSS JOIN LATERAL (
          SELECT member, source FROM (VALUES
              (1, CASE WHEN a.kind = 'capture_counterparty'
                        AND a.proposed_change ->> 'owner_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                       THEN (a.proposed_change ->> 'owner_id')::uuid END, 'payload_owner'),
              (2, CASE WHEN a.kind = 'commitment_task' THEN
                    CASE WHEN a.proposed_change ? 'existing_task_id'
                              AND a.proposed_change ->> 'seat_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                         THEN (a.proposed_change ->> 'seat_id')::uuid
                         WHEN a.proposed_change ->> 'promiser_seat_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                              AND (NOT a.proposed_change ? 'private_to'
                                   OR a.proposed_change ->> 'private_to' = a.proposed_change ->> 'promiser_seat_id')
                         THEN (a.proposed_change ->> 'promiser_seat_id')::uuid
                         -- Older proposals named the promiser only in seat_id.
                         -- Shared mail with no unresolved mark is attributable;
                         -- private mail is not, since its owner stood in.
                         WHEN NOT a.proposed_change ? 'promiser_seat_id'
                              AND coalesce(a.proposed_change ->> 'promiser_unresolved', 'false') <> 'true'
                              AND NOT a.proposed_change ? 'private_to'
                              AND a.proposed_change ->> 'seat_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                         THEN (a.proposed_change ->> 'seat_id')::uuid
                    END
                  ELSE a.on_behalf_of END, 'on_behalf_of'),
              (3, CASE WHEN a.kind <> 'commitment_task' AND a.proposed_by ~* '^human:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                       THEN substr(a.proposed_by, 7)::uuid END, 'stager'),
              (4, CASE WHEN a.kind = 'commitment_task' THEN NULL ELSE CASE a.target_entity_type
                       WHEN 'contact' THEN (SELECT owner_id FROM contact WHERE id = a.target_entity_id)
                       WHEN 'company' THEN (SELECT owner_id FROM company WHERE id = a.target_entity_id)
                       WHEN 'deal'    THEN (SELECT owner_id FROM deal    WHERE id = a.target_entity_id)
                       WHEN 'lead'    THEN (SELECT owner_id FROM lead    WHERE id = a.target_entity_id)
                       WHEN 'project' THEN (SELECT owner_id FROM project WHERE id = a.target_entity_id)
                       WHEN 'list'    THEN (SELECT owner_id FROM list    WHERE id = a.target_entity_id)
                   END END, 'record_owner')
          ) AS candidate(rank, member, source)
          WHERE member IS NOT NULL
            AND EXISTS (SELECT 1 FROM app_user u
                         WHERE u.id = candidate.member AND u.is_agent = false
                           AND u.status IN ('invited', 'active') AND u.archived_at IS NULL)
          ORDER BY rank
          LIMIT 1
      ) d
     WHERE a.status = 'pending'
)
UPDATE approval a
   SET requested_decider_id = resolved.member,
       requested_decider_source = resolved.source,
       -- A promise is decided by the seat it is staged for, so that seat must be
       -- the colleague it is now put to.
       on_behalf_of = CASE WHEN a.kind = 'commitment_task' THEN resolved.member ELSE a.on_behalf_of END
  FROM resolved
 WHERE a.id = resolved.id;

-- A pending request that names nobody is closed, with the reason on the row and
-- one audit entry each, the same terminal state the expiry sweep gives an
-- unanswered one.
WITH closed AS (
    UPDATE approval
       SET status = 'expired', decided_at = now(),
           decision_reason = 'nobody is responsible for this request: it names no member and its record has no owner',
           version = version + 1, updated_at = now()
     WHERE status = 'pending' AND requested_decider_id IS NULL
    RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'expire', 'approval', id,
       '{"status":"pending"}'::jsonb, '{"status":"expired"}'::jsonb
  FROM closed;

ALTER TABLE approval
    ADD CONSTRAINT approval_pending_names_decider
        CHECK (status <> 'pending' OR (requested_decider_id IS NOT NULL AND requested_decider_source IS NOT NULL)) NOT VALID;

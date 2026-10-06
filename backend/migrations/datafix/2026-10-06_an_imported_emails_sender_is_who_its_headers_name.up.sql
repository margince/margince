SET LOCAL lock_timeout = '3s';
-- The hand-logging writer used to record EVERY linked contact of a logged
-- email as its sender, whatever From/To/Cc the caller stated, so an email an
-- importer mirrored carries a `from` participant row per linked contact. The
-- writer is fixed; this corrects the ROLES of the rows already written: each
-- linked contact takes the role its address appears on in the stated headers.
-- Nothing is deleted and no address moves between rows — the fixed writer
-- keeps a contact row and the bare header rows side by side, and so does the
-- repair. A row whose role is already the header's is left exactly as it is,
-- which is what makes mail the fixed writer logged a no-op here.
--
-- Scope: email activities that state their headers (at least one
-- address-bearing from/to/cc row) AND carry at least one contact-only row,
-- which is the shape only the logged writer produces. An email without stated
-- headers keeps the old behaviour, exactly as the fixed writer does. An
-- activity where a stated address ever belonged to more than one contact is
-- left alone entirely: the address may have changed hands since capture, and
-- today's owner is not evidence about who sent mail then.

CREATE TEMP TABLE damaged_email ON COMMIT DROP AS
SELECT a.id, a.direction
  FROM activity a
 WHERE a.kind = 'email' AND a.archived_at IS NULL AND a.restricted_at IS NULL
   AND EXISTS (SELECT 1 FROM activity_participant h
                WHERE h.activity_id = a.id AND h.role IN ('from', 'to', 'cc')
                  AND h.address IS NOT NULL AND h.address <> '')
   AND EXISTS (SELECT 1 FROM activity_participant p
                WHERE p.activity_id = a.id AND p.role IN ('from', 'to', 'cc')
                  AND p.contact_id IS NOT NULL AND p.address IS NULL AND p.user_id IS NULL)
   AND NOT EXISTS (
         SELECT 1 FROM activity_participant h
          WHERE h.activity_id = a.id AND h.role IN ('from', 'to', 'cc')
            AND h.address IS NOT NULL
            AND (SELECT count(DISTINCT e.contact_id) FROM contact_email e
                  WHERE lower(e.email) = lower(h.address)) > 1);

-- What the rows said before, for the audit entry at the end.
CREATE TEMP TABLE before_rows ON COMMIT DROP AS
SELECT p.activity_id, p.role, p.contact_id, p.user_id, p.address
  FROM activity_participant p JOIN damaged_email d ON d.id = p.activity_id;

CREATE TEMP TABLE touched (activity_id uuid) ON COMMIT DROP;

-- Each contact-only row takes the role the stated headers give its contact:
-- the header a live address of theirs appears on, with the writer's own
-- precedence (from, then to, then cc); a contact the headers do not name
-- keeps the receiving side (cc on an inbound message, to otherwise).
CREATE TEMP TABLE header_role ON COMMIT DROP AS
SELECT d.id AS activity_id, p.id AS row_id, p.role AS old_role, p.contact_id,
       COALESCE((SELECT CASE WHEN bool_or(h.role = 'from') THEN 'from'
                             WHEN bool_or(h.role = 'to') THEN 'to'
                             WHEN bool_or(h.role = 'cc') THEN 'cc' END
                   FROM activity_participant h
                   JOIN contact_email e ON e.contact_id = p.contact_id
                                       AND e.archived_at IS NULL
                                       AND lower(e.email) = lower(h.address)
                  WHERE h.activity_id = d.id AND h.role IN ('from', 'to', 'cc')
                    AND h.address IS NOT NULL),
                CASE WHEN d.direction = 'inbound' THEN 'cc' ELSE 'to' END) AS new_role
  FROM damaged_email d
  JOIN activity_participant p ON p.activity_id = d.id
 WHERE p.contact_id IS NOT NULL AND p.address IS NULL AND p.user_id IS NULL
   AND p.role IN ('from', 'to', 'cc');

-- A row whose header role an identical row already holds is the duplicate the
-- pre-fix writer minted twice; the uniqueness index refuses the move, so it
-- goes instead.
WITH gone AS (
    DELETE FROM activity_participant p
     USING header_role hr
     WHERE p.id = hr.row_id AND hr.new_role <> hr.old_role
       AND EXISTS (SELECT 1 FROM activity_participant t
                    WHERE t.activity_id = hr.activity_id AND t.role = hr.new_role
                      AND t.contact_id = hr.contact_id AND t.address IS NULL AND t.user_id IS NULL)
    RETURNING p.activity_id
) INSERT INTO touched SELECT DISTINCT activity_id FROM gone;

WITH moved AS (
    UPDATE activity_participant p
       SET role = hr.new_role
      FROM header_role hr
     WHERE p.id = hr.row_id AND hr.new_role <> hr.old_role
       AND NOT EXISTS (SELECT 1 FROM activity_participant t
                        WHERE t.activity_id = hr.activity_id AND t.role = hr.new_role
                          AND t.contact_id = hr.contact_id AND t.address IS NULL AND t.user_id IS NULL)
    RETURNING p.activity_id
) INSERT INTO touched SELECT DISTINCT activity_id FROM moved;

INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'activity', t.activity_id,
       jsonb_build_object('participants',
         (SELECT jsonb_agg(jsonb_build_object('role', br.role, 'contact_id', br.contact_id,
                                              'user_id', br.user_id, 'address', br.address)
                           ORDER BY br.role, br.contact_id, br.user_id, br.address)
            FROM before_rows br WHERE br.activity_id = t.activity_id)),
       jsonb_build_object('participants',
         (SELECT jsonb_agg(jsonb_build_object('role', p.role, 'contact_id', p.contact_id,
                                              'user_id', p.user_id, 'address', p.address)
                           ORDER BY p.role, p.contact_id, p.user_id, p.address)
            FROM activity_participant p WHERE p.activity_id = t.activity_id),
         'datafix', '2026-10-06_an_imported_emails_sender_is_who_its_headers_name')
  FROM (SELECT DISTINCT activity_id FROM touched) t;

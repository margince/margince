SET LOCAL lock_timeout = '3s';
-- The hand-logging writer used to record EVERY linked contact of a logged
-- email as its sender, whatever From/To/Cc the caller stated, so an email an
-- importer mirrored carries a `from` participant row per linked contact. The
-- writer is fixed; this corrects the rows already written: each linked
-- contact takes the role its address appears on in the stated headers, and a
-- party described twice — once by contact or seat, once by bare address —
-- becomes one row.
--
-- Scope: email activities that state their headers (at least one
-- address-bearing from/to/cc row) AND carry at least one contact-only row,
-- which is the shape only the logged writer produces. An email without stated
-- headers keeps the old behaviour, exactly as the fixed writer does.

CREATE TEMP TABLE damaged_email ON COMMIT DROP AS
SELECT a.id, a.direction
  FROM activity a
 WHERE a.kind = 'email' AND a.archived_at IS NULL AND a.restricted_at IS NULL
   AND EXISTS (SELECT 1 FROM activity_participant h
                WHERE h.activity_id = a.id AND h.role IN ('from', 'to', 'cc')
                  AND h.address IS NOT NULL AND h.address <> '')
   AND EXISTS (SELECT 1 FROM activity_participant p
                WHERE p.activity_id = a.id AND p.role IN ('from', 'to', 'cc')
                  AND p.contact_id IS NOT NULL AND p.address IS NULL AND p.user_id IS NULL);

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

-- A row whose header role an identical row already holds is the duplicate.
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

-- A contact row keeps the stated address it stood beside, the way a promoted
-- capture row carries one.
WITH filled AS (
    UPDATE activity_participant k
       SET address = lower(b.address)
      FROM damaged_email d, activity_participant b, contact_email e
     WHERE k.activity_id = d.id AND b.activity_id = d.id
       AND k.contact_id IS NOT NULL AND k.user_id IS NULL AND k.address IS NULL
       AND b.user_id IS NULL AND b.contact_id IS NULL AND b.channel_user_id IS NULL
       AND b.address IS NOT NULL AND b.role = k.role
       AND e.contact_id = k.contact_id AND e.archived_at IS NULL
       AND lower(e.email) = lower(b.address)
       AND NOT EXISTS (SELECT 1 FROM activity_participant x
                        WHERE x.activity_id = k.activity_id AND x.role = k.role
                          AND x.contact_id = k.contact_id AND x.address = lower(b.address))
    RETURNING k.activity_id
) INSERT INTO touched SELECT DISTINCT activity_id FROM filled;

-- A contact-only row whose contact already has an address-bearing row on the
-- same header is the same party twice.
WITH gone AS (
    DELETE FROM activity_participant p
     USING damaged_email d
     WHERE p.activity_id = d.id
       AND p.contact_id IS NOT NULL AND p.address IS NULL AND p.user_id IS NULL
       AND EXISTS (SELECT 1 FROM activity_participant k
                    WHERE k.activity_id = d.id AND k.role = p.role
                      AND k.contact_id = p.contact_id AND k.address IS NOT NULL)
    RETURNING p.activity_id
) INSERT INTO touched SELECT DISTINCT activity_id FROM gone;

-- A bare address row whose party already sits on the same header as a contact
-- is the same party twice.
WITH gone AS (
    DELETE FROM activity_participant b
     USING damaged_email d
     WHERE b.activity_id = d.id
       AND b.user_id IS NULL AND b.contact_id IS NULL AND b.channel_user_id IS NULL
       AND b.address IS NOT NULL
       AND EXISTS (SELECT 1 FROM activity_participant k
                    WHERE k.activity_id = d.id AND k.role = b.role AND k.id <> b.id
                      AND k.contact_id IS NOT NULL
                      AND (lower(k.address) = lower(b.address)
                           OR EXISTS (SELECT 1 FROM contact_email e
                                       WHERE e.contact_id = k.contact_id AND e.archived_at IS NULL
                                         AND lower(e.email) = lower(b.address))))
    RETURNING b.activity_id
) INSERT INTO touched SELECT DISTINCT activity_id FROM gone;

-- A bare address row naming a seat that already sits on the same header as a
-- user row is the same human twice.
WITH gone AS (
    DELETE FROM activity_participant b
     USING damaged_email d, activity_participant k
     WHERE b.activity_id = d.id
       AND b.user_id IS NULL AND b.contact_id IS NULL AND b.channel_user_id IS NULL
       AND b.address IS NOT NULL
       AND k.activity_id = d.id AND k.role = b.role AND k.user_id IS NOT NULL
       AND (EXISTS (SELECT 1 FROM app_user u
                     WHERE u.id = k.user_id AND lower(u.email) = lower(b.address))
            OR EXISTS (SELECT 1 FROM capture_owner_identity oi
                        WHERE oi.user_id = k.user_id AND oi.kind = 'address'
                          AND oi.value = lower(b.address)))
    RETURNING b.activity_id
) INSERT INTO touched SELECT DISTINCT activity_id FROM gone;

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

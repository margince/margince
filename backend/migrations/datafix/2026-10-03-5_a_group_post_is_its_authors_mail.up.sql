SET LOCAL lock_timeout = '3s';
-- A Google Group rewrites From to itself, so a post an outsider sent to a shared
-- address was captured with the group as its counterparty and its sender row,
-- and the group's own List-Unsubscribe marked it bulk mail. Capture now reads
-- the author (mailmap.googleGroupSender); this corrects the posts captured
-- before. Posts dropped as internal were never stored and are not here.
--
-- A group post is recognised as capture recognises it, on the stored original's
-- unfolded head: an X-Google-Group-Id, and a From that is exactly the group its
-- List-ID names. Its author is the X-Original-From address. A post stays bulk
-- when any List-Unsubscribe link points anywhere but Google Groups, judged by
-- the link's own host or mailto domain. A post its seat wrote is left alone.
WITH head AS (
    SELECT a.id, a.counterparty_email, a.bulk_mail_attested, ci.user_id AS seat,
           regexp_replace(substring(rc.payload #>> '{}' FROM '^(.*?)\r?\n\r?\n'), '\r?\n[ \t]+', ' ', 'g') AS h
      FROM activity a
      JOIN raw_capture rc ON rc.id = a.raw_capture_id
      JOIN capture_import ci ON ci.activity_id = a.id
     WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL
       AND jsonb_typeof(rc.payload) = 'string'
       AND left(rc.payload #>> '{}', 65536) ~* E'\nX-Google-Group-Id:'
),
parsed AS (
    SELECT id, counterparty_email, bulk_mail_attested, seat, h,
           lower(coalesce((regexp_match(h, E'(?:^|\n)From:[^\n]*<([^>\n]+@[^>\n]+)>', 'i'))[1],
                          btrim((regexp_match(h, E'(?:^|\n)From:\\s*([^\\s<>\n]+@[^\\s<>\n]+)', 'i'))[1]))) AS group_address,
           lower(btrim((regexp_match(h, E'(?:^|\n)List-ID:[^\n]*<([^>\n]+)>', 'i'))[1])) AS list_id,
           lower(coalesce((regexp_match(h, E'(?:^|\n)X-Original-From:[^\n]*<([^>\n]+@[^>\n]+)>', 'i'))[1],
                          btrim((regexp_match(h, E'(?:^|\n)X-Original-From:\\s*([^\\s<>\n]+@[^\\s<>\n]+)', 'i'))[1]))) AS author
      FROM head
),
posts AS (
    SELECT p.id, p.counterparty_email, p.bulk_mail_attested, p.seat, p.group_address, p.author,
           -- Bulk stays only where a link names somewhere but Google Groups.
           EXISTS (
             SELECT 1
               FROM regexp_matches(p.h, E'(?:^|\n)List-Unsubscribe:([^\n]*)', 'gi') AS lu(v),
                    regexp_matches(lu.v[1], '<([^>]+)>', 'g') AS link(u)
              WHERE NOT (lower(link.u[1]) ~ '^mailto:[^?]*@googlegroups\.com(\?|$)'
                         OR lower(link.u[1]) ~ '^https://groups\.google\.com(/|:|\?|$)')
           ) AS own_unsubscribe
      FROM parsed p
     WHERE p.group_address IS NOT NULL AND p.author IS NOT NULL
       AND p.list_id = replace(p.group_address, '@', '.')
       AND p.author <> p.group_address
       AND NOT EXISTS (SELECT 1 FROM capture_owner_identity oi
                        WHERE oi.user_id = p.seat AND oi.kind = 'address' AND oi.value = p.author)
       AND NOT EXISTS (SELECT 1 FROM capture_connection cc
                        WHERE cc.user_id = p.seat AND lower(cc.account_label) = p.author)
),
changed AS (
    UPDATE activity a
       SET counterparty_email = posts.author,
           bulk_mail_attested = posts.bulk_mail_attested AND posts.own_unsubscribe
      FROM posts
     WHERE a.id = posts.id
       AND (a.counterparty_email IS DISTINCT FROM posts.author
            OR a.bulk_mail_attested IS DISTINCT FROM (posts.bulk_mail_attested AND posts.own_unsubscribe))
    RETURNING a.id
),
sender AS (
    UPDATE activity_participant ap
       SET address = posts.author
      FROM posts
     WHERE ap.activity_id = posts.id AND ap.role = 'from' AND ap.user_id IS NULL AND ap.contact_id IS NULL
       AND ap.address = posts.group_address
       AND NOT EXISTS (SELECT 1 FROM activity_participant x
                        WHERE x.activity_id = posts.id AND x.role = 'from' AND x.address = posts.author)
    RETURNING ap.activity_id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'activity', posts.id,
       jsonb_build_object('counterparty_email', posts.counterparty_email, 'bulk_mail_attested', posts.bulk_mail_attested),
       jsonb_build_object('counterparty_email', posts.author,
                          'bulk_mail_attested', posts.bulk_mail_attested AND posts.own_unsubscribe,
                          'datafix', '2026-10-03-5_a_group_post_is_its_authors_mail')
  FROM posts JOIN changed ON changed.id = posts.id;

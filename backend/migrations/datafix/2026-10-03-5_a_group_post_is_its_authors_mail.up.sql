SET LOCAL lock_timeout = '3s';
-- A Google Group rewrites From to itself, so a post an outsider sent to a shared
-- address was captured with the group as its counterparty and its sender row,
-- and the group's own List-Unsubscribe marked it bulk mail. Capture now reads
-- the author (mailmap.googleGroupSender); this corrects the posts captured
-- before. Posts dropped as internal were never stored and are not here.
--
-- SQL cannot run the Go parser, so it acts only where the stored head has the
-- one plain shape that parser reads the same way, and leaves anything else as
-- captured: exactly one X-Google-Group-Id (non-blank), one From naming one
-- address, one X-Original-From naming one address, and one List-ID that is a
-- bare <id> equal to the From address with its @ turned into a dot. A post is
-- left alone when any seat that imported it is its author. Its counterparty
-- moves only from the group to the author, and it stays bulk unless every
-- List-Unsubscribe token is provably Google's own link.
WITH head AS (
    SELECT a.id, a.counterparty_email, a.bulk_mail_attested,
           E'\n' || regexp_replace(substring(rc.payload #>> '{}' FROM '^(.*?)\r?\n\r?\n'), '\r?\n[ \t]+', ' ', 'g') AS h
      FROM activity a
      JOIN raw_capture rc ON rc.id = a.raw_capture_id
     WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL
       AND jsonb_typeof(rc.payload) = 'string'
       AND left(rc.payload #>> '{}', 65536) ~* E'(^|\n)X-Google-Group-Id:'
),
fields AS (
    SELECT id, counterparty_email, bulk_mail_attested, h,
           (SELECT array_agg(btrim(m[1])) FROM regexp_matches(h, E'\nX-Google-Group-Id:([^\r\n]*)', 'gi') m) AS group_ids,
           (SELECT array_agg(btrim(m[1])) FROM regexp_matches(h, E'\nFrom:([^\r\n]*)', 'gi') m) AS froms,
           (SELECT array_agg(btrim(m[1])) FROM regexp_matches(h, E'\nX-Original-From:([^\r\n]*)', 'gi') m) AS originals,
           (SELECT array_agg(btrim(m[1])) FROM regexp_matches(h, E'\nList-ID:([^\r\n]*)', 'gi') m) AS list_ids
      FROM head
),
-- One address per field, in either plain shape: `addr` or `"Name" <addr>` with
-- no second address after it. Anything else reads as NULL and is left alone.
single AS (
    SELECT id, counterparty_email, bulk_mail_attested, h,
           lower(coalesce((regexp_match(froms[1], '^(?:"[^"]*"|[^"<>,]*)\s*<([^<>,\s]+@[^<>,\s]+)>$'))[1],
                          (regexp_match(froms[1], '^([^<>,"\s]+@[^<>,"\s]+)$'))[1])) AS group_address,
           lower(coalesce((regexp_match(originals[1], '^(?:"[^"]*"|[^"<>,]*)\s*<([^<>,\s]+@[^<>,\s]+)>$'))[1],
                          (regexp_match(originals[1], '^([^<>,"\s]+@[^<>,"\s]+)$'))[1])) AS author,
           lower((regexp_match(list_ids[1], '^<([^<>\s]+)>$'))[1]) AS list_id
      FROM fields
     WHERE cardinality(group_ids) = 1 AND group_ids[1] <> ''
       AND cardinality(froms) = 1 AND cardinality(originals) = 1 AND cardinality(list_ids) = 1
),
posts AS (
    SELECT s.id, s.counterparty_email, s.bulk_mail_attested, s.group_address, s.author,
           -- Bulk stays unless every unsubscribe token is Google's own.
           EXISTS (
             SELECT 1
               FROM regexp_matches(s.h, E'\nList-Unsubscribe:([^\r\n]*)', 'gi') AS lu(v),
                    regexp_split_to_table(lu.v[1], ',') AS token(t)
              WHERE btrim(token.t) <> ''
                AND NOT (lower(btrim(btrim(token.t), '<>')) ~ '^mailto:[^@?<>\s]+@googlegroups\.com(\?[^<>\s]*)?$'
                         OR lower(btrim(btrim(token.t), '<>')) ~ '^https://groups\.google\.com(/[^<>\s]*)?$')
           ) AS own_unsubscribe
      FROM single s
     WHERE s.group_address IS NOT NULL AND s.author IS NOT NULL
       AND s.list_id = replace(s.group_address, '@', '.')
       AND s.author <> s.group_address
       AND s.counterparty_email IN (s.group_address, s.author)
       -- Left alone when ANY seat that imported it is its author.
       AND NOT EXISTS (
             SELECT 1 FROM capture_import ci
              WHERE ci.activity_id = s.id
                AND (EXISTS (SELECT 1 FROM capture_owner_identity oi
                              WHERE oi.user_id = ci.user_id AND oi.kind = 'address' AND oi.value = s.author)
                     OR EXISTS (SELECT 1 FROM capture_connection cc
                                 WHERE cc.user_id = ci.user_id AND lower(cc.account_label) = s.author)))
       AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = s.id)
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
-- The group's sender row becomes the author's, or goes where the author
-- already has one.
dropped AS (
    DELETE FROM activity_participant ap
     USING posts
     WHERE ap.activity_id = posts.id AND ap.role = 'from' AND ap.address = posts.group_address
       AND ap.user_id IS NULL AND ap.contact_id IS NULL
       AND EXISTS (SELECT 1 FROM activity_participant x
                    WHERE x.activity_id = posts.id AND x.role = 'from' AND x.address = posts.author)
    RETURNING ap.activity_id
),
renamed AS (
    UPDATE activity_participant ap
       SET address = posts.author
      FROM posts
     WHERE ap.activity_id = posts.id AND ap.role = 'from' AND ap.address = posts.group_address
       AND ap.user_id IS NULL AND ap.contact_id IS NULL
       AND NOT EXISTS (SELECT 1 FROM activity_participant x
                        WHERE x.activity_id = posts.id AND x.role = 'from' AND x.address = posts.author)
    RETURNING ap.activity_id
),
affected AS (
    SELECT id FROM changed
    UNION SELECT activity_id FROM dropped
    UNION SELECT activity_id FROM renamed
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'activity', posts.id,
       jsonb_build_object('counterparty_email', posts.counterparty_email, 'bulk_mail_attested', posts.bulk_mail_attested,
                          'sender', posts.group_address),
       jsonb_build_object('counterparty_email', posts.author,
                          'bulk_mail_attested', posts.bulk_mail_attested AND posts.own_unsubscribe,
                          'sender', posts.author,
                          'datafix', '2026-10-03-5_a_group_post_is_its_authors_mail')
  FROM posts JOIN affected ON affected.id = posts.id;

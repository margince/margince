-- The growth loop of poc/worklist-proposal.md 1.6: one more year of mail per call.
-- shape 'same_people' keeps old mail on today's contacts; 'new_people' clones them.
-- Run on a copy of the bench data, never on a database that matters.

ALTER TABLE activity_participant ADD COLUMN IF NOT EXISTS occurred_at timestamptz;
CREATE TABLE IF NOT EXISTS poc_grow_src AS SELECT id FROM activity WHERE kind = 'email';
CREATE TABLE IF NOT EXISTS poc_grow_years (year int PRIMARY KEY, shape text, mails bigint, done_at timestamptz DEFAULT now());
CREATE OR REPLACE FUNCTION public.poc_cols(t regclass, prefix text DEFAULT ''::text)
 RETURNS text
 LANGUAGE sql
AS $function$
  SELECT string_agg(prefix || quote_ident(attname), ',' ORDER BY attnum) FROM pg_attribute
   WHERE attrelid = t AND attnum > 0 AND NOT attisdropped AND attgenerated = '' $function$
;
CREATE OR REPLACE FUNCTION public.poc_grow_year(k integer, shape text)
 RETURNS bigint
 LANGUAGE plpgsql
AS $function$
DECLARE n bigint; shift interval := make_interval(years => k);
BEGIN
  SET LOCAL session_replication_role = replica;
  CREATE TEMP TABLE amap ON COMMIT DROP AS SELECT id AS orig, gen_random_uuid() AS fresh FROM poc_grow_src;
  CREATE TEMP TABLE cmap (orig uuid PRIMARY KEY, fresh uuid) ON COMMIT DROP;
  IF shape = 'new_people' THEN
    INSERT INTO cmap SELECT DISTINCT c, gen_random_uuid() FROM (
      SELECT l.contact_id c FROM activity_link l JOIN poc_grow_src s ON s.id = l.activity_id WHERE l.contact_id IS NOT NULL
      UNION SELECT p.contact_id FROM activity_participant p JOIN poc_grow_src s ON s.id = p.activity_id WHERE p.contact_id IS NOT NULL) x;
    CREATE TEMP TABLE cbatch ON COMMIT DROP AS SELECT c.* FROM contact c JOIN cmap m ON m.orig = c.id;
    UPDATE cbatch b SET id = m.fresh, created_at = b.created_at - shift,
      source_id = CASE WHEN b.source_id IS NULL THEN NULL ELSE b.source_id || '-y' || k END
      FROM cmap m WHERE m.orig = b.id;
    EXECUTE format('INSERT INTO contact (%s) SELECT %s FROM cbatch', poc_cols('contact'), poc_cols('contact'));
  END IF;
  CREATE TEMP TABLE abatch ON COMMIT DROP AS SELECT a.* FROM activity a JOIN poc_grow_src s ON s.id = a.id;
  UPDATE abatch b SET id = m.fresh, occurred_at = b.occurred_at - shift, created_at = b.created_at - shift,
    source_id = CASE WHEN b.source_id IS NULL THEN NULL ELSE b.source_id || '-y' || k END,
    thread_key = CASE WHEN b.thread_key IS NULL THEN NULL ELSE b.thread_key || '-y' || k END
    FROM amap m WHERE m.orig = b.id;
  EXECUTE format('INSERT INTO activity (%s) SELECT %s FROM abatch', poc_cols('activity'), poc_cols('activity'));
  GET DIAGNOSTICS n = ROW_COUNT;
  CREATE TEMP TABLE pbatch ON COMMIT DROP AS SELECT p.* FROM activity_participant p JOIN poc_grow_src s ON s.id = p.activity_id;
  UPDATE pbatch b SET id = gen_random_uuid(), activity_id = m.fresh, occurred_at = b.occurred_at - shift,
    created_at = b.created_at - shift FROM amap m WHERE m.orig = b.activity_id;
  UPDATE pbatch b SET contact_id = m.fresh FROM cmap m WHERE m.orig = b.contact_id;
  EXECUTE format('INSERT INTO activity_participant (%s) SELECT %s FROM pbatch', poc_cols('activity_participant'), poc_cols('activity_participant'));
  CREATE TEMP TABLE lbatch ON COMMIT DROP AS SELECT l.* FROM activity_link l JOIN poc_grow_src s ON s.id = l.activity_id
    WHERE l.entity_type IN ('contact', 'company');
  UPDATE lbatch b SET id = gen_random_uuid(), activity_id = m.fresh, created_at = b.created_at - shift FROM amap m WHERE m.orig = b.activity_id;
  UPDATE lbatch b SET contact_id = m.fresh FROM cmap m WHERE m.orig = b.contact_id;
  EXECUTE format('INSERT INTO activity_link (%s) SELECT %s FROM lbatch', poc_cols('activity_link'), poc_cols('activity_link'));
  INSERT INTO poc_grow_years (year, shape, mails) VALUES (k, shape, n);
  RETURN n;
END $function$
;

-- usage: SELECT poc_grow_year(k, 'new_people') FROM generate_series(1, 7) k; then VACUUM ANALYZE.

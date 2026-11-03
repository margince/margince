SET LOCAL lock_timeout = '3s';
DO $$ BEGIN
    RAISE EXCEPTION 'Superseded: do not run this repair. Use 2026-11-03-2_replies_share_meaningful_topic_words.up.sql after reviewing its preview.';
END $$;

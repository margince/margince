SET LOCAL lock_timeout = '2s';
ALTER TABLE public.activity_reader_state VALIDATE CONSTRAINT activity_reader_state_intent_pair;
ALTER TABLE public.brief_item VALIDATE CONSTRAINT brief_item_intent_pair;

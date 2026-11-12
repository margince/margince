SET LOCAL lock_timeout = '2s';
ALTER TABLE public.activity VALIDATE CONSTRAINT activity_meeting_intent_pair;
ALTER TABLE public.activity VALIDATE CONSTRAINT activity_meeting_intent_kind;

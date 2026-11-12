SET LOCAL lock_timeout = '2s';
DROP TRIGGER meeting_intent_insert ON public.activity;
DROP TRIGGER meeting_intent_update ON public.activity;
ALTER TABLE public.activity DROP CONSTRAINT activity_meeting_intent_pair, DROP CONSTRAINT activity_meeting_intent_kind,
 DROP COLUMN meeting_wall, DROP COLUMN meeting_zone;

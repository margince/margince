DROP TRIGGER activity_intent_insert ON public.activity;
DROP TRIGGER activity_intent_update ON public.activity;
ALTER TABLE public.activity DROP CONSTRAINT activity_due_intent_pair, DROP CONSTRAINT activity_reminder_intent_pair,
 DROP COLUMN due_wall, DROP COLUMN due_zone, DROP COLUMN remind_wall, DROP COLUMN remind_zone;

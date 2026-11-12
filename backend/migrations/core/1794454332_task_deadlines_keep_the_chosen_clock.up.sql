ALTER TABLE public.activity
 ADD COLUMN due_wall timestamp without time zone,
 ADD COLUMN due_zone text,
 ADD COLUMN remind_wall timestamp without time zone,
 ADD COLUMN remind_zone text,
 ADD CONSTRAINT activity_due_intent_pair CHECK (public.valid_intent_time(due_wall, due_zone)) NOT VALID,
 ADD CONSTRAINT activity_reminder_intent_pair CHECK (public.valid_intent_time(remind_wall, remind_zone)) NOT VALID;
CREATE TRIGGER activity_intent_insert BEFORE INSERT ON public.activity
 FOR EACH ROW WHEN (NEW.due_wall IS NOT NULL OR NEW.remind_wall IS NOT NULL)
 EXECUTE FUNCTION public.maintain_intent_times(
 'due_wall', 'due_zone', 'due_at', 'remind_wall', 'remind_zone', 'remind_at');
CREATE TRIGGER activity_intent_update BEFORE UPDATE OF due_wall, due_zone, due_at, remind_wall, remind_zone, remind_at ON public.activity
 FOR EACH ROW WHEN (NEW.due_wall IS NOT NULL OR NEW.remind_wall IS NOT NULL OR OLD.due_wall IS NOT NULL OR OLD.remind_wall IS NOT NULL)
 EXECUTE FUNCTION public.maintain_intent_times(
 'due_wall', 'due_zone', 'due_at', 'remind_wall', 'remind_zone', 'remind_at');

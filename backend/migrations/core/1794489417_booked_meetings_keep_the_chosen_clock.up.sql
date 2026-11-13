SET LOCAL lock_timeout = '2s';
ALTER TABLE public.activity
 ADD COLUMN meeting_wall timestamp without time zone,
 ADD COLUMN meeting_zone text,
 ADD CONSTRAINT activity_meeting_intent_pair CHECK (public.valid_intent_time(meeting_wall, meeting_zone)) NOT VALID,
 ADD CONSTRAINT activity_meeting_intent_kind CHECK (meeting_wall IS NULL OR (kind = 'meeting' AND occurred_precision = 'instant')) NOT VALID;
CREATE TRIGGER meeting_intent_insert BEFORE INSERT ON public.activity
 FOR EACH ROW WHEN (NEW.meeting_wall IS NOT NULL)
 EXECUTE FUNCTION public.maintain_intent_times('meeting_wall', 'meeting_zone', 'occurred_at');
CREATE TRIGGER meeting_intent_update BEFORE UPDATE OF meeting_wall, meeting_zone, occurred_at, meeting_status ON public.activity
 FOR EACH ROW WHEN ((NEW.meeting_wall IS NOT NULL OR OLD.meeting_wall IS NOT NULL) AND
  (NEW.meeting_status IS NULL OR NEW.meeting_status = 'booked' OR OLD.meeting_status IS NULL OR OLD.meeting_status = 'booked'
   OR NEW.meeting_wall IS DISTINCT FROM OLD.meeting_wall OR NEW.meeting_zone IS DISTINCT FROM OLD.meeting_zone
   OR NEW.occurred_at IS DISTINCT FROM OLD.occurred_at))
 EXECUTE FUNCTION public.maintain_intent_times('meeting_wall', 'meeting_zone', 'occurred_at');

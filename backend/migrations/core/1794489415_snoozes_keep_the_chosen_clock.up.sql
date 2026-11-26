SET LOCAL lock_timeout = '2s';
DO $pairs$
DECLARE target record; wall_column text; zone_column text;
BEGIN
 FOR target IN SELECT * FROM (VALUES
  ('activity_reader_state', 'snoozed', 'snoozed_until'),
  ('brief_item', 'snoozed', 'snoozed_until')
 ) AS pairs(table_name, prefix, instant_column)
 LOOP
  wall_column := target.prefix || '_wall';
  zone_column := target.prefix || '_zone';
  EXECUTE format('ALTER TABLE public.%I ADD COLUMN %I timestamp without time zone,
   ADD COLUMN %I text, ADD CONSTRAINT %I CHECK (public.valid_intent_time(%I, %I)) NOT VALID',
   target.table_name, wall_column, zone_column, target.table_name || '_intent_pair', wall_column, zone_column);
  EXECUTE format('CREATE TRIGGER snooze_intent_insert BEFORE INSERT ON public.%I
   FOR EACH ROW WHEN (NEW.%I IS NOT NULL) EXECUTE FUNCTION public.maintain_intent_times(%L, %L, %L)',
   target.table_name, wall_column, wall_column, zone_column, target.instant_column);
  EXECUTE format('CREATE TRIGGER snooze_intent_update BEFORE UPDATE OF %I, %I, %I ON public.%I
   FOR EACH ROW WHEN (NEW.%I IS NOT NULL OR OLD.%I IS NOT NULL)
   EXECUTE FUNCTION public.maintain_intent_times(%L, %L, %L)',
   wall_column, zone_column, target.instant_column, target.table_name,
   wall_column, wall_column, wall_column, zone_column, target.instant_column);
 END LOOP;
END
$pairs$;

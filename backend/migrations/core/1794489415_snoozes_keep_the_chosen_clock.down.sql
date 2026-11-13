SET LOCAL lock_timeout = '2s';
DO $pairs$
DECLARE target record;
BEGIN
 FOR target IN SELECT * FROM (VALUES
  ('activity_reader_state', 'snoozed'), ('brief_item', 'snoozed')
 ) AS pairs(table_name, prefix)
 LOOP
  EXECUTE format('DROP TRIGGER snooze_intent_insert ON public.%I', target.table_name);
  EXECUTE format('DROP TRIGGER snooze_intent_update ON public.%I', target.table_name);
  EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I, DROP COLUMN %I, DROP COLUMN %I',
   target.table_name, target.table_name || '_intent_pair', target.prefix || '_wall', target.prefix || '_zone');
 END LOOP;
END
$pairs$;

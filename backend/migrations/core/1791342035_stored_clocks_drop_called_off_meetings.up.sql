-- Refolds the stored last_activity_at of every company, contact and deal whose
-- value moved under 1791342034's rule, so a clock a called-off meeting set
-- before that migration stops pointing at it.
--
-- A file of its own so it runs after 1791342034 has committed and released its
-- lock on activity: this transaction holds only the row locks of the records
-- it rewrites. Those are taken in the one (kind, id) order every writer uses
-- (lock_last_activity_targets: company, then contact, then deal), so a
-- concurrent activity or employment write queues behind it rather than
-- deadlocking.
--
-- Through move_last_activity, so a clock move bumps no version and stamps no
-- updated_at. Only records whose value moves are written.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE
  r record;
BEGIN
  FOR r IN
    SELECT id FROM company
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_company(id)
     ORDER BY id
  LOOP
    PERFORM move_last_activity('company'::regclass, r.id);
  END LOOP;
  FOR r IN
    SELECT id FROM contact
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_contact(id)
     ORDER BY id
  LOOP
    PERFORM move_last_activity('contact'::regclass, r.id);
  END LOOP;
  FOR r IN
    SELECT id FROM deal
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_deal(id)
     ORDER BY id
  LOOP
    PERFORM move_last_activity('deal'::regclass, r.id);
  END LOOP;
END $$;

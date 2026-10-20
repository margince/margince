-- The seeded rules that destroy a record just because it is old are switched off.
--
-- A fresh installation is no longer planted with `activity/transcript` (erase
-- after 365 days) or `contact/no_consent_no_deal` (anonymize after 730 days):
-- each removes, for good and on age alone, what was said or who someone is. An
-- installation bootstrapped earlier still holds the two rows, and this turns
-- them off where every column still holds the seeded value. A row an admin changed in any
-- column is their decision and stays as it is. The rows are disabled rather
-- than deleted, so the settings page still shows the rule and an admin can turn
-- it back on.
--
-- `raw_capture` keeps the window it has here. The seed now plants 90 days, and
-- shortening a running installation's window would erase originals on the next
-- sweep that no admin chose to erase.
SET LOCAL lock_timeout = '3s';

UPDATE retention_policy
   SET enabled = false
 WHERE enabled
   AND lawful_basis IS NOT DISTINCT FROM 'storage_limitation'
   AND ((object_type = 'activity' AND category = 'transcript' AND retain_days = 365 AND action = 'erase')
     OR (object_type = 'contact' AND category = 'no_consent_no_deal' AND retain_days = 730 AND action = 'anonymize'));

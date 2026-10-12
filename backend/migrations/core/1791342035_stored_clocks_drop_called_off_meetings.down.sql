-- Nothing to restore. The up migration rewrote stored values to what the
-- functions answered, and this runs before 1791342034's down restores the old
-- rule, so a refold here would write the same values again. Each record's
-- clock refolds under the restored rule as its activities next change.
SELECT 1;

-- Back to the bound that admits an empty week. A row carrying `{}` would pass
-- either constraint, so this direction needs no repair first.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user DROP CONSTRAINT app_user_work_days_are_weekdays;

ALTER TABLE app_user ADD CONSTRAINT app_user_work_days_are_weekdays
    CHECK (work_days IS NULL
           OR (array_length(work_days, 1) BETWEEN 1 AND 7
               AND work_days <@ ARRAY[1,2,3,4,5,6,7]::smallint[]));

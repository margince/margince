-- Neither can fail: the column is new and every row carries its constant default.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_occurred_precision_check;
ALTER TABLE activity VALIDATE CONSTRAINT activity_day_holds_no_host_slot;

-- Bounded like the up migration: dropping a table takes ACCESS EXCLUSIVE, and
-- without this a rollback behind a long transaction waits for that lock while
-- every reader and writer queues behind it.
SET LOCAL lock_timeout = '3s';

-- Items first: the cycle's own drop would be refused by the foreign key from
-- the table above it.
DROP TABLE IF EXISTS assurance_task_item;
DROP TABLE IF EXISTS assurance_cycle;

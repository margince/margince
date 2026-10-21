-- Nothing to undo. Turning the rows back on cannot tell a row this migration
-- disabled from one an admin disabled since, and the next sweep would destroy
-- records on a rule nobody chose to restore.
SELECT 1;

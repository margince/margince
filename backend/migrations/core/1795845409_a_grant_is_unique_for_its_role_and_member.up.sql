-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_role_assignment_member;
CREATE UNIQUE INDEX CONCURRENTLY uq_role_assignment_member ON role_assignment (role_id, user_id);

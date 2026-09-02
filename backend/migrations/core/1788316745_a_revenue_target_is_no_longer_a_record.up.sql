-- The human-set sales quota is retired: Margince no longer stores a revenue
-- target, and no longer divides closed-won by one to publish an attainment.
--
-- A quota was a number a manager typed. Everything the product said about
-- performance downstream of it -- attainment, the pace band, the leaderboard --
-- was that number's arithmetic, so a figure nobody had entered read as an
-- absence rather than as an unanswerable question. Founder decision: the
-- product does not manage revenue targets. Pipeline sufficiency against
-- historical conversion is a different question with its own denominator, and
-- it is not this table under another name.
--
-- TWO STATEMENTS, ONE RETIREMENT. The grant strip and the DROP belong in one
-- migration because they are one fact: the object stops being grantable in the
-- same release the table stops existing. Splitting them would leave a window
-- where a role still carries a grant on a table nobody can read.
--
-- WHY THE GRANT STRIP REACHES EVERY ROLE, not just the seeded six.
-- policy.Parse drops an object outside coreObjects when it reads a document, so
-- a stale key breaks nothing at runtime. But identity's ListRoles returns the
-- STORED objects verbatim, so an operator-defined role would keep displaying a
-- quota grant that no screen can clear and no policy can honour. The predicate
-- below names the object rather than the roles, and every document holding it
-- loses it.
--
-- ROLLBACK IS LOSSY, and the down half says so where it does the work: the
-- rows are gone with the table, and a custom role's original grant cannot be
-- reconstructed because this migration did not record what it was.
--
-- ROLLING DEPLOY. The api applies migrations at its entrypoint and then serves
-- (docs/deployment.md), so during a multi-replica api rollout an old replica
-- can still answer GET /quotas after this runs, and it will fault on a missing
-- relation. That is accepted here rather than deferred to a second release: the
-- surface being faulted is the one this release deletes, the fault is a read
-- returning an error for the length of a rollout, and there is no write path
-- and nothing to corrupt. The two-step alternative keeps a dead table and a
-- tableownership entry naming a module that no longer exists. If a future
-- retirement's stale code could CORRUPT rather than fault, that trade goes the
-- other way -- this reasoning is about this table.
--
-- DROP takes an exclusive lock on a table this migration did not create.
-- Bounded so a transaction holding a conflicting lock fails this migration fast
-- rather than stalling every write to that table for as long as it would queue.
SET LOCAL lock_timeout = '3s';

UPDATE role
   SET permissions = permissions #- '{objects,quota}'
 WHERE permissions ? 'objects'
   AND (permissions -> 'objects') ? 'quota';

DROP TABLE IF EXISTS quota;

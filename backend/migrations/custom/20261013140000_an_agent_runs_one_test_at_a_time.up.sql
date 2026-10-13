SET LOCAL lock_timeout = '3s';

-- An agent holds at most one test run at a time; its second preview is told
-- which run it already has. A person is bounded by the queue instead. A plain
-- build, because the table holds one row per import.
CREATE UNIQUE INDEX import_run_one_test_per_agent ON import_run ((actor->>'id'))
    WHERE status = 'validating' AND actor->>'type' = 'agent';

# Rebuild the indexes of a queue table

A queue table is small and written all day: every row is inserted, updated a few times and deleted.
Its indexes can grow to many times the size of its rows. Autovacuum cleans these tables after 2% of
their rows change, which slows the growth. It does not shrink an index that has already grown. A
rebuild does.

The tables are `river_job`, `event_outbox`, `idempotency_key`, `capture_trace`,
`graph_interaction_edge` and `activity_participant`.

Nothing in Margince rebuilds these indexes. The worker runs under the app role, which may not run
DDL, so the queue's own rebuild is switched off (`backend/internal/platform/jobs/jobs.go`).

## Check whether a rebuild is due

Connect with the owner DSN and compare each table with its indexes:

```sql
SELECT c.relname AS table_name,
       pg_size_pretty(pg_table_size(c.oid)) AS rows_size,
       pg_size_pretty(pg_indexes_size(c.oid)) AS indexes_size
  FROM pg_class c
 WHERE c.relname IN ('river_job', 'event_outbox', 'idempotency_key', 'capture_trace',
                     'graph_interaction_edge', 'activity_participant');
```

Rebuild a table whose indexes are several times the size of its rows.

## Rebuild

1. Connect as the owner role. The app role is refused.
2. Run the rebuild for one table at a time:

   ```sql
   REINDEX TABLE CONCURRENTLY river_job;
   ```

   `CONCURRENTLY` lets the app read and write the table while the rebuild runs. It cannot run inside a
   transaction, so send the statement by itself.
3. Run the size query again. The size of the indexes is now near the size of the rows.

A rebuild that is stopped leaves an index marked `INVALID`, with a name that ends in `_ccnew`. Drop
that index and run the rebuild again.

<!-- prose:plain -->
# Measure a mail import

Run these tests from the repository root with the test services running. They use test mail with
the real parser, sink and stores, on a test database.

## Capture work

```sh
make test-it DIR=backend/internal/compose/integration/capture RUN=TestHistoryImportReportsItsDatabaseWork
```

The test checks the run and its creation ledger. It prints the statement count, connection requests,
database time and pool wait. Set `MARGINCE_IMPORT_BENCH_MESSAGES` to choose the message count.
The test checks the range in
[`capture_backfill_cost_integration_test.go`](../../backend/internal/compose/integration/capture/capture_backfill_cost_integration_test.go).
Use the same count and configuration when comparing revisions.

The report shows each part of the run. Compare the last parts with the first ones
to check whether the database work per message grows with the imported history.

Set `MARGINCE_IMPORT_BENCH_SQL=1` to print costs by statement. With that option,
`MARGINCE_IMPORT_BENCH_PREPARED=1` uses a separate pool with cached statements.
The pool settings are in
[`capture_sqlprofile_integration_test.go`](../../backend/internal/compose/integration/capture/capture_sqlprofile_integration_test.go).
The test pool uses no statement cache so tests may change the schema.
Use the prepared pool to measure cached queries. Record the selected mode with the result.

Set `MARGINCE_IMPORT_BENCH_PLAN_DIR` to save plans for the costly reads and name candidates.

The test ends when capture ends. It does not run background model or relationship jobs.
Measure those jobs on their own before reporting total time.

## Thread lookup

```sh
make test-it DIR=backend/internal/compose RUN=TestMailNeighboursSeekPastUnrelatedHistory
```

The test calls `MailNeighboursTx` with test history. It prints the Postgres plan and its database
work. Check the returned IDs too: a change in the plan alone does not show a lower import time.

Keep revision IDs, configuration and logs outside the source tree. Run each test again under the
same load. The tests check results; their time may change between runs.

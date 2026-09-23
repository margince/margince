// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The purge's count-then-compare, driven where the state it guards against can
// actually be reached.
//
// PurgeRawCaptureTx asks how many of its records name an original before it
// destroys them, and refuses when the delete reaches fewer. The schema makes
// that disagreement unreachable through Postgres: activity.raw_capture_id is a
// foreign key with ON DELETE SET NULL, so a reference to a row that is gone
// cannot exist while the constraint stands. An integration test would have to
// drop the constraint to produce one — on a schema every other test in the
// process shares, restored by a cleanup that a failure can skip, leaving the
// rest of the run against a schema production does not have.
//
// So the tx is the boundary this drives (P3): the fake answers the count and
// the delete independently, which is the one way to hold the comparison to
// both verdicts.

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestThePurgeRefusesWhenFewerOriginalsDieThanAreNamed(t *testing.T) {
	tx := &countingTx{named: 2, deleted: 1}

	err := (&PendingStore{}).PurgeRawCaptureTx(context.Background(), tx, []ids.UUID{ids.NewV7(), ids.NewV7()})

	if err == nil {
		t.Fatal("the purge reported success over a record whose original it did not destroy")
	}
	if !strings.Contains(err.Error(), "these records name 2 provider original(s) and 1 were destroyed") {
		t.Errorf("the refusal does not say which side came up short: %v", err)
	}
}

func TestThePurgeAcceptsWhenEveryNamedOriginalDies(t *testing.T) {
	tx := &countingTx{named: 2, deleted: 2}

	if err := (&PendingStore{}).PurgeRawCaptureTx(context.Background(), tx, []ids.UUID{ids.NewV7(), ids.NewV7()}); err != nil {
		t.Fatalf("the purge refused a batch it destroyed in full: %v", err)
	}
}

// TestThePurgeAcceptsTwoRecordsSharingOneOriginal is why the count is DISTINCT:
// the delete destroys rows, so one row dying for two records is a finished
// purge, and a count of records would refuse it.
func TestThePurgeAcceptsTwoRecordsSharingOneOriginal(t *testing.T) {
	tx := &countingTx{named: 1, deleted: 1}

	if err := (&PendingStore{}).PurgeRawCaptureTx(context.Background(), tx, []ids.UUID{ids.NewV7(), ids.NewV7()}); err != nil {
		t.Fatalf("the purge refused two records whose one shared original it destroyed: %v", err)
	}
}

// TestThePurgeAcceptsABatchWithNoOriginalToDestroy holds the other end: a
// record naming no original is not an unfinished purge. Nothing to destroy and
// nothing destroyed agree, and a comparison that read that as a shortfall would
// refuse every hand-typed record in a redaction batch.
func TestThePurgeAcceptsABatchWithNoOriginalToDestroy(t *testing.T) {
	tx := &countingTx{named: 0, deleted: 0}

	if err := (&PendingStore{}).PurgeRawCaptureTx(context.Background(), tx, []ids.UUID{ids.NewV7()}); err != nil {
		t.Fatalf("the purge refused a batch with no original behind it: %v", err)
	}
}

// countingTx is the true-DB-boundary fake (P3), in the shape
// storekit/emitevent_test.go's fakeTx uses: the two calls PurgeRawCaptureTx
// makes answer from the fixture, and every other pgx.Tx method panics —
// reaching one would be this test's own bug rather than a path worth stubbing.
type countingTx struct {
	named   int64
	deleted int64
}

func (f *countingTx) QueryRow(context.Context, string, ...any) pgx.Row { return countRow(f.named) }

func (f *countingTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("DELETE " + strconv.FormatInt(f.deleted, 10)), nil
}

// countRow answers the one scan the purge makes of its count.
type countRow int64

func (r countRow) Scan(dest ...any) error {
	into, ok := dest[0].(*int64)
	if !ok {
		panic("countingTx: the purge scanned its count into something other than an int64")
	}
	*into = int64(r)
	return nil
}

func (f *countingTx) Begin(context.Context) (pgx.Tx, error) {
	panic("countingTx: Begin not implemented")
}
func (f *countingTx) Commit(context.Context) error   { panic("countingTx: Commit not implemented") }
func (f *countingTx) Rollback(context.Context) error { panic("countingTx: Rollback not implemented") }

func (f *countingTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("countingTx: CopyFrom not implemented")
}

func (f *countingTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("countingTx: SendBatch not implemented")
}

func (f *countingTx) LargeObjects() pgx.LargeObjects {
	panic("countingTx: LargeObjects not implemented")
}

func (f *countingTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("countingTx: Prepare not implemented")
}

func (f *countingTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("countingTx: Query not implemented")
}
func (f *countingTx) Conn() *pgx.Conn { panic("countingTx: Conn not implemented") }

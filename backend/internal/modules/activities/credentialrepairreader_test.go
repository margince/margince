// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// systemCtx is the principal this repair runs as. It reads and writes every
// row it is given, so no narrower principal reaches the code below.
func systemCtx() context.Context {
	return principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})
}

// A row the read cannot reach is skipped, not reported.
//
// The repair is given ids from a page it read earlier. A row restricted or
// deleted since is no longer this pass's to mask.
//
// Reporting it would fail a run over state that is already correct.
func TestARowTheRepairCannotReadAnyMoreIsSkipped(t *testing.T) {
	got, err := MaskStoredMailTextTx(systemCtx(),
		unreachableTx{err: pgx.ErrNoRows}, []ids.ActivityID{ids.ActivityID{UUID: ids.NewV7()}})
	if err != nil {
		t.Fatalf("a vanished row was reported: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a vanished row was counted as masked: %v", got)
	}
}

// A read that fails for any other reason stops the pass.
//
// A read the pass cannot complete must not be recorded as a row with nothing
// to mask.
func TestAFailedReadStopsTheRepair(t *testing.T) {
	refused := errors.New("the connection went away")
	_, err := MaskStoredMailTextTx(systemCtx(),
		unreachableTx{err: refused}, []ids.ActivityID{ids.ActivityID{UUID: ids.NewV7()}})
	if !errors.Is(err, refused) {
		t.Fatalf("the failure was not reported: %v", err)
	}
	if !strings.Contains(err.Error(), "reading the stored text") {
		t.Errorf("the error does not say what failed: %v", err)
	}
}

// Only the system principal runs this. A repair reachable by a signed-in seat
// would let one rewrite stored mail text outside every ordinary write path.
func TestOnlyTheSystemRunsTheRepair(t *testing.T) {
	seat := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, ID: "user:a-seat"})
	if _, err := MaskStoredMailTextTx(seat,
		unreachableTx{err: pgx.ErrNoRows},
		[]ids.ActivityID{ids.ActivityID{UUID: ids.NewV7()}}); err == nil {
		t.Fatal("a human seat ran the stored-text repair")
	}
}

// A column that holds nothing keeps holding nothing.
//
// Masking a NULL subject into an empty string would turn "this mail has no
// subject" into "this mail's subject is blank".
//
// The repair has no reason to decide that, and the write guard would see it as
// a change.
func TestMaskingAnAbsentColumnLeavesItAbsent(t *testing.T) {
	if got := maskedPtr(nil); got != nil {
		t.Errorf("a NULL column masked to %q", *got)
	}
	typed := "Passwort: Hunter2-Example!"
	masked := maskedPtr(&typed)
	if masked == nil {
		t.Fatal("a column holding a credential masked to NULL")
	}
	if strings.Contains(*masked, "Hunter2") {
		t.Errorf("the credential survived masking: %q", *masked)
	}
}

// The write guard compares the text it read with the text in the row, and two
// absent columns are the same text. Reading them as different would make every
// mail with no subject look like a row that moved under the lock.
func TestTwoAbsentColumnsCompareEqual(t *testing.T) {
	blank := ""
	held := "a subject"
	for _, c := range []struct {
		name string
		a, b *string
		want bool
	}{
		{"both absent", nil, nil, true},
		{"absent against blank", nil, &blank, false},
		{"absent against held", nil, &held, false},
		{"held against itself", &held, &held, true},
		{"blank against held", &blank, &held, false},
	} {
		if got := same(c.a, c.b); got != c.want {
			t.Errorf("%s: same() = %v, want %v", c.name, got, c.want)
		}
	}
}

// countingTx is a database that answers the repair's UPDATE with a row count of
// its own choosing. Anything else reaching it fails the test.
//
// The repair pins its UPDATE on the text it read, so a count of zero means the
// row moved under the lock.
type countingTx struct {
	pgx.Tx
	affected int64
	execSQL  []string
}

func (tx *countingTx) Exec(
	_ context.Context, sql string, _ ...any,
) (pgconn.CommandTag, error) {
	tx.execSQL = append(tx.execSQL, sql)
	return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tx.affected)), nil
}

// A row whose text moved under the lock is left for the next pass, and records
// nothing.
//
// An audit row here would say a field changed from the text this pass read,
// and that is not what the row held.
//
// The next pass reads the row again and masks whatever it holds then.
func TestARowThatMovedUnderTheLockRecordsNothing(t *testing.T) {
	typed := "Passwort: Hunter2-Example!"
	tx := &countingTx{affected: 0}
	wrote, err := maskRowTextTx(systemCtx(), tx,
		ids.ActivityID{UUID: ids.NewV7()}, nil, &typed)
	if err != nil {
		t.Fatalf("a row that moved was reported as a failure: %v", err)
	}
	if wrote {
		t.Error("a row that moved was counted as masked")
	}
	if len(tx.execSQL) != 1 {
		t.Fatalf("the repair ran %d statements, want only the pinned update", len(tx.execSQL))
	}
	if !strings.Contains(tx.execSQL[0], "UPDATE activity") {
		t.Errorf("the one statement was not the update: %q", tx.execSQL[0])
	}
}

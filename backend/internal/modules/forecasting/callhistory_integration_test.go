// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package forecasting

// The chain a call leaves behind, read back.
//
// Each case works in its OWN period, and the distinct years are load-bearing.
// An installation holds exactly one workspace, so these tests share a database:
// two cases calling the same (period, scope) would see each other's rows, and
// the count assertions would fail on a collision rather than on a defect.
//
// They run in sequence, not under t.Parallel(). The fixture they share ends
// every case with testdb.AssertPoolsQuiesced, which asks whether ANY connection
// of the package's shared pool is still out — a question a sibling case still
// mid-transaction answers with a false leak, and it did, on a loaded runner,
// against whichever case happened to finish first.
//
// These need real Postgres for the reason the snapshot tests do: the ordering
// is SQL's, the scope match is SQL's, and the NULL-versus-value distinction
// that decides whether a workspace call finds its own predecessor cannot be
// exercised against Go alone. A unit test over this package would assert that
// a query string was built and prove nothing about what it returns.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// callAt records one call and returns it, failing the test rather than the
// caller so each case reads as the thing it is about.
func callAt(t *testing.T, e *snapshotEnv, ctx context.Context, period Period, scope Scope, minor int64) Call {
	t.Helper()
	call, err := e.store.RecordCall(ctx, NewCall{
		Period: period, Scope: scope, AmountMinor: minor, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("recording a call of %d: %v", minor, err)
	}
	return call
}

// historyOf reads a period's calls the way production does — inside a
// transaction, through the same unexported read the handler uses. There is no
// exported store method to call: the read is not row-scoped by itself, so it
// stays behind the handler that resolves the caller's scope first.
func historyOf(t *testing.T, e *snapshotEnv, ctx context.Context, period Period, scope Scope) ([]Call, error) {
	t.Helper()
	var out []Call
	err := e.store.InTx(ctx, func(inner context.Context, tx pgx.Tx) error {
		var readErr error
		out, readErr = e.store.callHistoryTx(inner, tx, period, scope)
		return readErr
	})
	return out, err
}

// quarterOf resolves the quarter containing one day, in the zone the
// installation counts days in.
func quarterOf(t *testing.T, day time.Time) Period {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	period, err := ResolvePeriod(PeriodQuarter, day.In(zone), 1, zone)
	if err != nil {
		t.Fatal(err)
	}
	return period
}

// The history is the whole point: three calls made in sequence come back as
// three, newest first, each naming the one it replaced.
//
// The first call of a period names NO predecessor, which is a different fact
// from replacing nothing — a reader walking the chain needs the end of it to
// be reachable.
func TestTheCallHistoryIsTheChainNewestFirst(t *testing.T) {
	e := setupSnapshot(t)
	ctx := e.as()
	period := quarterOf(t, time.Date(2031, time.August, 12, 12, 0, 0, 0, time.UTC))
	scope := Scope{Kind: ScopeWorkspace}

	first := callAt(t, e, ctx, period, scope, 2_400_000)
	second := callAt(t, e, ctx, period, scope, 1_900_000)
	third := callAt(t, e, ctx, period, scope, 2_100_000)

	got, err := historyOf(t, e, ctx, period, scope)
	if err != nil {
		t.Fatalf("reading the history: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("history holds %d calls, want the 3 that were made", len(got))
	}
	wantOrder := []int64{2_100_000, 1_900_000, 2_400_000}
	for i, want := range wantOrder {
		if got[i].AmountMinor != want {
			t.Errorf("history[%d] is %d, want %d — the order is newest first",
				i, got[i].AmountMinor, want)
		}
	}
	if got[0].ID != third.ID {
		t.Error("the newest entry is not the call made last")
	}
	if got[0].SupersedesID == nil || *got[0].SupersedesID != second.ID {
		t.Error("the newest call does not name the one it replaced")
	}
	if got[2].SupersedesID != nil {
		t.Errorf("the first call of the period names a predecessor (%v), and there was none",
			*got[2].SupersedesID)
	}
	if got[2].ID != first.ID {
		t.Error("the oldest entry is not the call made first")
	}
}

// A period nobody has called is an empty history and no error. It is a real
// answer about a period the caller may read, and the endpoint serves `[]`
// rather than `null` so a reader cannot tell one from the other by accident.
func TestAPeriodNobodyCalledHasAnEmptyHistory(t *testing.T) {
	e := setupSnapshot(t)
	period := quarterOf(t, time.Date(2033, time.November, 12, 12, 0, 0, 0, time.UTC))

	got, err := historyOf(t, e, e.as(), period, Scope{Kind: ScopeWorkspace})
	if err != nil {
		t.Fatalf("reading an uncalled period: %v — an uncalled period is an answer, not an error", err)
	}
	if len(got) != 0 {
		t.Fatalf("an uncalled period holds %d calls", len(got))
	}
	if got == nil {
		t.Error("an uncalled period answered nil, which serves `null` rather than `[]`")
	}
}

// One period's history is not another's, and one scope's is not another's.
//
// The scope match is the half that would fail quietly: scope_id is NULL for
// the workspace, and a query written with `=` instead of IS NOT DISTINCT FROM
// finds nothing for the one scope every installation has.
func TestACallHistoryHoldsOnlyItsOwnPeriodAndScope(t *testing.T) {
	e := setupSnapshot(t)
	ctx := e.as()
	q3 := quarterOf(t, time.Date(2032, time.September, 12, 12, 0, 0, 0, time.UTC))
	q4 := quarterOf(t, time.Date(2032, time.December, 12, 12, 0, 0, 0, time.UTC))
	team := ids.NewV7()

	callAt(t, e, ctx, q3, Scope{Kind: ScopeWorkspace}, 1_000_000)
	callAt(t, e, ctx, q4, Scope{Kind: ScopeWorkspace}, 2_000_000)
	callAt(t, e, ctx, q3, Scope{Kind: ScopeTeam, ID: &team}, 3_000_000)

	workspaceQ3, err := historyOf(t, e, ctx, q3, Scope{Kind: ScopeWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceQ3) != 1 || workspaceQ3[0].AmountMinor != 1_000_000 {
		t.Errorf("the workspace's Q3 history holds %d calls, want only its own",
			len(workspaceQ3))
	}

	teamQ3, err := historyOf(t, e, ctx, q3, Scope{Kind: ScopeTeam, ID: &team})
	if err != nil {
		t.Fatal(err)
	}
	if len(teamQ3) != 1 || teamQ3[0].AmountMinor != 3_000_000 {
		t.Errorf("the team's Q3 history holds %d calls, want only its own", len(teamQ3))
	}
}

// A reader who may not read forecasts at all reads no history.
//
// The gate is InTx's, which every path into this read goes through — the
// handler opens the transaction there too. The read itself is unexported and
// ungated on purpose: it is not row-scoped by itself, so it stays behind the
// handler that resolves the caller's scope before calling it, and a second
// object check here would refuse a manager who may call but not read.
func TestACallHistoryNeedsTheForecastReadGrant(t *testing.T) {
	e := setupSnapshot(t)
	period := quarterOf(t, time.Date(2034, time.August, 12, 12, 0, 0, 0, time.UTC))

	if _, err := historyOf(t, e, e.asUngranted(), period, Scope{Kind: ScopeWorkspace}); err == nil {
		t.Fatal("a reader without forecast:read was served the call history")
	}
}

// asUngranted is a seat with no forecast grant at all — the reader the object
// gate exists to refuse.
func (e *snapshotEnv) asUngranted() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
			RowScope: principal.RowScopeOwn,
		},
	})
}

// The cap takes the NEWEST calls, not the oldest.
//
// Which end it cuts is the whole question. A cap that kept the oldest entries
// would answer a reviewer's "how did this period move" with the first few
// numbers anyone ever said and none of the recent ones — the opposite of what
// they asked, while still looking like a full answer.
//
// A period is called past the cap here rather than at it, so the assertion is
// about the cut and not about an off-by-one at the boundary.
func TestACappedHistoryKeepsTheNewestCalls(t *testing.T) {
	e := setupSnapshot(t)
	ctx := e.as()
	period := quarterOf(t, time.Date(2035, time.May, 12, 12, 0, 0, 0, time.UTC))
	scope := Scope{Kind: ScopeWorkspace}

	// Each call's amount records the order it was made in, so the entries that
	// survive the cap say which end was kept.
	const past = callHistoryLimit + 3
	for i := 1; i <= past; i++ {
		callAt(t, e, ctx, period, scope, int64(i))
	}

	got, err := historyOf(t, e, ctx, period, scope)
	if err != nil {
		t.Fatalf("reading the history: %v", err)
	}
	if len(got) != callHistoryLimit {
		t.Fatalf("history holds %d calls, want the cap of %d", len(got), callHistoryLimit)
	}
	if got[0].AmountMinor != int64(past) {
		t.Errorf("the newest entry is call %d, want the last one made (%d) — "+
			"the cap kept the wrong end", got[0].AmountMinor, past)
	}
	// The oldest SURVIVOR, which is the cap counted back from the newest call.
	if want := int64(past - callHistoryLimit + 1); got[len(got)-1].AmountMinor != want {
		t.Errorf("the oldest surviving entry is call %d, want %d",
			got[len(got)-1].AmountMinor, want)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package forecasting

// Which snapshots a reading lists is decided in SQL: the period and scope it
// matches, the population it excludes, the cap and the first-of-period anchor.
// Each test takes its own period year because the database is shared across the
// package's tests and a snapshot taken for one would be listed by another.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func yearQuarter(t *testing.T, year int) Period {
	t.Helper()
	period, err := ResolvePeriod(PeriodQuarter, time.Date(year, time.February, 14, 12, 0, 0, 0, time.UTC), 1, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	return period
}

// freeze takes a recheck snapshot, the trigger the daily arbiter leaves
// unconstrained, so a test can take as many as it needs.
func (e *snapshotEnv) freeze(t *testing.T, in NewSnapshot) ids.UUID {
	t.Helper()
	in.Trigger, in.BaseCurrency = TriggerRecheck, "EUR"
	in.Readings = Readings{Contributions: []Contribution{}}
	var id ids.UUID
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = e.store.TakeSnapshot(ctx, tx, in)
		return err
	}); err != nil {
		t.Fatalf("freezing a snapshot: %v", err)
	}
	return id
}

func (e *snapshotEnv) refs(t *testing.T, period Period, scope Scope) []SnapshotRef {
	t.Helper()
	var out []SnapshotRef
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = e.store.SnapshotRefsTx(ctx, tx, period, scope)
		return err
	}); err != nil {
		t.Fatalf("listing the snapshots: %v", err)
	}
	return out
}

func TestAPeriodNobodyFroze(t *testing.T) {
	t.Parallel()
	e := setupSnapshot(t)
	refs := e.refs(t, yearQuarter(t, 2041), Scope{Kind: ScopeWorkspace})
	if refs == nil || len(refs) != 0 {
		t.Fatalf("a period with nothing frozen listed %v, want an empty non-nil list", refs)
	}
}

func TestSnapshotsListNewestFirstWithoutRepeatingTheFirst(t *testing.T) {
	t.Parallel()
	e := setupSnapshot(t)
	period, scope := yearQuarter(t, 2042), Scope{Kind: ScopeWorkspace}
	base := time.Date(2042, time.February, 1, 9, 0, 0, 0, time.UTC)
	var taken []ids.UUID
	for i := range 3 {
		taken = append(taken, e.freeze(t, NewSnapshot{
			Period: period, Scope: scope, TakenAt: base.Add(time.Duration(i) * time.Hour),
		}))
	}

	refs := e.refs(t, period, scope)
	if len(refs) != 3 {
		t.Fatalf("listed %d snapshots, want the 3 taken — the first is among the newest and must appear once", len(refs))
	}
	for i, ref := range refs {
		if want := taken[len(taken)-1-i]; ref.ID != want {
			t.Errorf("position %d is %s, want %s — newest first", i, ref.ID, want)
		}
		if ref.Trigger != TriggerRecheck {
			t.Errorf("position %d carries trigger %q, want %q", i, ref.Trigger, TriggerRecheck)
		}
	}
}

func TestAPeriodWithManySnapshotsListsTheNewestTenAndTheFirst(t *testing.T) {
	t.Parallel()
	e := setupSnapshot(t)
	period, scope := yearQuarter(t, 2043), Scope{Kind: ScopeWorkspace}
	base := time.Date(2043, time.February, 1, 0, 0, 0, 0, time.UTC)
	var taken []ids.UUID
	for i := range 13 {
		taken = append(taken, e.freeze(t, NewSnapshot{
			Period: period, Scope: scope, TakenAt: base.Add(time.Duration(i) * time.Hour),
		}))
	}

	refs := e.refs(t, period, scope)
	if len(refs) != recentSnapshotRefs+1 {
		t.Fatalf("listed %d snapshots, want the newest %d plus the period's first", len(refs), recentSnapshotRefs)
	}
	for i := range recentSnapshotRefs {
		if want := taken[len(taken)-1-i]; refs[i].ID != want {
			t.Errorf("position %d is %s, want %s", i, refs[i].ID, want)
		}
	}
	if last := refs[len(refs)-1]; last.ID != taken[0] {
		t.Errorf("the anchor is %s, want the period's first snapshot %s", last.ID, taken[0])
	}
}

// A snapshot of another population is not a state of this one, and
// differencing across them would report the population change as movement.
func TestSnapshotsOfAnotherPopulationAreNotListed(t *testing.T) {
	t.Parallel()
	e := setupSnapshot(t)
	ctx := context.Background()
	period, scope := yearQuarter(t, 2044), Scope{Kind: ScopeWorkspace}
	at := time.Date(2044, time.February, 1, 9, 0, 0, 0, time.UTC)

	pipeline := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `INSERT INTO pipeline (id, name) VALUES ($1, 'Restricted')`, pipeline); err != nil {
		t.Fatal(err)
	}
	whole := e.freeze(t, NewSnapshot{Period: period, Scope: scope, TakenAt: at})
	e.freeze(t, NewSnapshot{Period: period, Scope: scope, TakenAt: at.Add(time.Hour), PipelineID: &pipeline})
	e.freeze(t, NewSnapshot{
		Period: period, Scope: scope, TakenAt: at.Add(2 * time.Hour), PopulationFingerprint: "fixed-population",
	})
	owner := e.rep
	e.freeze(t, NewSnapshot{
		Period: period, Scope: Scope{Kind: ScopeOwner, ID: &owner}, TakenAt: at.Add(3 * time.Hour),
	})
	e.freeze(t, NewSnapshot{Period: yearQuarter(t, 2045), Scope: scope, TakenAt: at.Add(4 * time.Hour)})

	refs := e.refs(t, period, scope)
	if len(refs) != 1 || refs[0].ID != whole {
		t.Fatalf("listed %v, want only the whole-pipeline workspace snapshot %s", refs, whole)
	}
	ownerRefs := e.refs(t, period, Scope{Kind: ScopeOwner, ID: &owner})
	if len(ownerRefs) != 1 {
		t.Fatalf("the owner scope listed %d snapshots, want its own 1", len(ownerRefs))
	}
}

// The REST door reads the same store method the tool does, so what it serves is
// pinned here against the real handler rather than assumed from the method.
func TestTheForecastEndpointListsThePeriodsSnapshots(t *testing.T) {
	t.Parallel()
	e := setupSnapshot(t)
	period, scope := yearQuarter(t, 2046), Scope{Kind: ScopeWorkspace}
	id := e.freeze(t, NewSnapshot{
		Period: period, Scope: scope, TakenAt: time.Date(2046, time.February, 1, 9, 0, 0, 0, time.UTC),
	})

	handlers := NewHandlers(e.store,
		func(context.Context, pgx.Tx, Period, Scope, time.Time, string) ([]Deal, Scope, bool, error) {
			return nil, scope, false, nil
		},
		func(context.Context, pgx.Tx, PeriodKind, time.Time) (Period, string, error) {
			return period, "EUR", nil
		},
		nil,
		func(context.Context, pgx.Tx, Period, Scope, time.Time, string) (ConversionHistory, error) {
			return ConversionHistory{}, nil
		},
		func(context.Context, pgx.Tx) (ForwardMeasure, error) { return values.MeasureCommitEvidence, nil },
		func() time.Time { return time.Date(2046, time.February, 14, 12, 0, 0, 0, time.UTC) },
	)
	rec := httptest.NewRecorder()
	handlers.GetForecast(rec, httptest.NewRequest(http.MethodGet, "/v1/forecast", nil).WithContext(e.as()),
		crmcontracts.GetForecastParams{})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/forecast answered %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Snapshots []struct {
			ID      string `json:"id"`
			Trigger string `json:"trigger"`
		} `json:"snapshots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Snapshots) != 1 || body.Snapshots[0].ID != id.String() || body.Snapshots[0].Trigger != TriggerRecheck {
		t.Fatalf("the endpoint served snapshots %+v, want the one frozen (%s)", body.Snapshots, id)
	}
}

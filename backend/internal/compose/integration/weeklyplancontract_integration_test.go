// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// What the rep says about the week ahead, over real migrated Postgres.
//
// The three things a unit test cannot see: the write leaves an audit row and an
// event behind, a closed week refuses the write, and the capacity line counts
// only the meetings that are actually next week's.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func ptr(s string) *string { return &s }

// THE WRITE SHAPE. Domain row, audit row and event in ONE transaction — the
// obligation every mutation in this tree carries, and a plan's prose is a
// mutation like any other.
func TestRisksAndCapacityNoteAreAuditedAndEmitted(t *testing.T) {
	e := setupPlan(t)

	plan, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{
		SetRisks: true, Risks: ptr("Two contacts out; the Nordwind renewal has no sponsor"),
		SetCapacityNote: true, CapacityNote: ptr("Conference Thursday and Friday"),
	})
	if err != nil {
		t.Fatalf("writing the contract: %v", err)
	}
	if plan.Risks == nil || plan.CapacityNote == nil {
		t.Fatalf("the plan must carry what was just written, got %+v", plan)
	}

	var audits, events int
	if err := database.WithWorkspaceTx(e.rep1Ctx, e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(e.rep1Ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'weekly_plan' AND entity_id = $1
			   AND action = 'update'
			   AND after->>'risks' IS NOT NULL`, plan.ID).Scan(&audits); err != nil {
			return err
		}
		// The event's TYPE and its changed-field list, not merely that a row
		// exists: a consumer filters on both, and an event naming the wrong
		// field reaches nobody who is listening for this change.
		return tx.QueryRow(e.rep1Ctx, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->>'type' = 'weekly_plan.updated'
			   AND envelope->'payload'->>'plan_id' = $1::text
			   AND envelope->'payload'->'changed_fields' ? 'contract'`,
			plan.ID).Scan(&events)
	}); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	if audits == 0 {
		t.Error("the contract write left no audit row naming the risks it stored")
	}
	if events == 0 {
		t.Error("the contract write left no event, so no consumer learns the plan changed")
	}
}

// A CLOSED WEEK IS HISTORY. Its counts are frozen into a review that has been
// read; a plan that kept accepting prose would let a rep rewrite what their
// lead already saw.
func TestAClosedWeeksContractCannotBeRewritten(t *testing.T) {
	e := setupPlan(t)

	if _, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{SetRisks: true, Risks: ptr("As planned"), SetCapacityNote: true, CapacityNote: nil}); err != nil {
		t.Fatalf("writing the contract while the week is open: %v", err)
	}
	// Closing runs the week on, exactly as the weekly job does.
	if _, err := e.store.CloseWeek(e.rep1Ctx, planClock.AddDate(0, 0, 7)); err != nil {
		t.Fatalf("closing the week: %v", err)
	}

	_, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{SetRisks: true, Risks: ptr("Rewritten after the fact"), SetCapacityNote: true, CapacityNote: nil})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "week_closed" {
		t.Fatalf("rewriting a closed week got %v, wanted a week_closed refusal", err)
	}
}

// AN ABSENT SEAM IS UNKNOWN, NEVER ZERO. An installation that composed no
// calendar has not looked at next week; reporting "nothing booked" would tell a
// rep their week is free on the strength of a missing integration.
func TestAnAbsentCapacitySeamLeavesCapacityAbsentNotZero(t *testing.T) {
	e := setupPlan(t)
	// setupPlan builds the store WITHOUT WithCapacity, which is the state under
	// test: no calendar reader composed.
	if _, err := e.store.StartWeek(e.rep1Ctx, planClock); err != nil {
		t.Fatal(err)
	}
	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Capacity != nil {
		t.Fatalf("no capacity seam composed must read as unknown, got %+v", plan.Capacity)
	}
}

// stubCapacity answers a fixed load, so the test above and this one differ in
// exactly one thing: whether a reader is composed at all.
//
// It RECORDS the week it was asked about. A stub that ignored the argument
// would answer the same figure whichever week the store named, so a call site
// passing the wrong one — the defect this seam was reshaped to end — would run
// green through every test here.
type stubCapacity struct {
	committed weeklyplan.Committed
	askedFor  *time.Time
}

func (s stubCapacity) ForWeek(_ context.Context, _ ids.UUID, weekStart time.Time) (weeklyplan.Committed, error) {
	if s.askedFor != nil {
		*s.askedFor = weekStart
	}
	return s.committed, nil
}

// A COMPOSED SEAM IS A FIGURE, including a real zero. The mirror of the case
// above: a rep whose next week is genuinely empty gets 0, and that 0 is a fact
// about their calendar rather than about the installation.
func TestAComposedCapacitySeamReportsEvenAnEmptyWeek(t *testing.T) {
	e := setupPlan(t)
	e.store = e.store.WithCapacity(stubCapacity{})
	if _, err := e.store.StartWeek(e.rep1Ctx, planClock); err != nil {
		t.Fatal(err)
	}
	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Capacity == nil {
		t.Fatal("a composed seam answers a figure, even when the week is empty")
	}
	if plan.Capacity.Meetings != 0 || plan.Capacity.Tasks != 0 {
		t.Fatalf("the stub reports an empty week, got %+v", plan.Capacity)
	}
}

// AN OMITTED FIELD IS NOT A CLEARED ONE. A client sending only `risks` must not
// silently erase a capacity note somebody wrote — the two halves travel
// together and each keeps its own state.
func TestWritingOneHalfOfTheContractLeavesTheOtherAlone(t *testing.T) {
	e := setupPlan(t)
	if _, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{
		SetRisks: true, Risks: ptr("Sponsor risk"),
		SetCapacityNote: true, CapacityNote: ptr("Conference Thursday"),
	}); err != nil {
		t.Fatal(err)
	}
	// Only the risks half is SET. The capacity note is left unsent, and the
	// store resolves it from the row under its own lock — which is the whole
	// point: passing the value read a moment ago would race a concurrent save.
	plan, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{
		SetRisks: true, Risks: ptr("Sponsor risk, now escalated"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CapacityNote == nil || *plan.CapacityNote != "Conference Thursday" {
		t.Fatalf("the untouched half must stand, got %v", plan.CapacityNote)
	}
}

// CLEARED IS NOT UNWRITTEN. A rep who empties the field has said there is
// nothing to name; one who never wrote has said nothing. Both reach the
// surface, so both must survive a round trip.
func TestAClearedRiskIsTellableFromOneNeverWritten(t *testing.T) {
	e := setupPlan(t)
	if _, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{SetRisks: true, Risks: ptr("Something"), SetCapacityNote: true, CapacityNote: nil}); err != nil {
		t.Fatal(err)
	}
	cleared, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{SetRisks: true, Risks: ptr(""), SetCapacityNote: true, CapacityNote: nil})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Risks == nil {
		t.Fatal("an emptied field is present and empty, not absent")
	}
	if *cleared.Risks != "" {
		t.Fatalf("wanted an empty string, got %q", *cleared.Risks)
	}
	unset, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{SetRisks: true, Risks: nil, SetCapacityNote: true, CapacityNote: nil})
	if err != nil {
		t.Fatal(err)
	}
	if unset.Risks != nil {
		t.Fatalf("a nil clears the column back to unwritten, got %q", *unset.Risks)
	}
}

// A SUCCESSFUL SAVE ANSWERS WITH THE CAPACITY A READ WOULD GIVE. The plan's own
// tables do not carry it — the seam does — so a write that returned only what
// readPlan knows would tell a client "no calendar composed" moments after a GET
// told it otherwise.
func TestASavedContractStillCarriesTheCountedCapacity(t *testing.T) {
	e := setupPlan(t)
	var priced time.Time
	e.store = e.store.WithCapacity(stubCapacity{
		committed: weeklyplan.Committed{Meetings: 3, Tasks: 2},
		askedFor:  &priced,
	})

	plan, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{
		SetRisks: true, Risks: ptr("Sponsor risk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The week PRICED is the week STORED. Read from the clock instead, the
	// capacity line described a different seven days from the plan it sat on.
	if !priced.Equal(plan.LocalWeekStart) {
		t.Errorf("capacity priced the week of %s for a plan stored against %s",
			priced.Format(time.DateOnly), plan.LocalWeekStart.Format(time.DateOnly))
	}
	if plan.Capacity == nil {
		t.Fatal("a save must answer with the capacity a read would give, not omit it")
	}
	if plan.Capacity.Meetings != 3 || plan.Capacity.Tasks != 2 {
		t.Fatalf("wanted the composed figure back, got %+v", plan.Capacity)
	}
}

// PROSE IS BOUNDED IN CHARACTERS, NOT BYTES. The refusal says "characters" and
// the column's CHECK counts characters, so a note in German or Vietnamese must
// not hit a ceiling the same length in English clears.
func TestProseIsBoundedInCharactersSoAccentsDoNotCostDouble(t *testing.T) {
	e := setupPlan(t)
	// 1500 accented characters — well inside 2000 — but 3000 BYTES.
	long := strings.Repeat("é", 1500)

	plan, err := e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{
		SetRisks: true, Risks: &long,
	})
	if err != nil {
		t.Fatalf("1500 characters is inside the 2000-character bound: %v", err)
	}
	if plan.Risks == nil || utf8.RuneCountInString(*plan.Risks) != 1500 {
		t.Fatalf("the whole note must be stored, got %d characters",
			utf8.RuneCountInString(deref(plan.Risks)))
	}

	// And the bound still bites: 2001 characters is refused.
	tooLong := strings.Repeat("é", 2001)
	_, err = e.store.SetContract(e.rep1Ctx, planClock, weeklyplan.ContractEdit{
		SetRisks: true, Risks: &tooLong,
	})
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "too_long" {
		t.Fatalf("2001 characters must be refused as too_long, got %v", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

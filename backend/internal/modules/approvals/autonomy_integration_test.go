// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The autonomy ladder against a real database.
//
// Everything worth proving here is what the schema and the transaction hold:
// that a decision and the counter it earns commit together, that the three
// outcomes land in three different columns, that one rep's record is invisible
// to another, and that the constraints refuse a policy the product cannot keep.
// None of it can be shown without Postgres.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countedKind is a server-composed kind with decision grants, so a decision on
// it reaches the counter. Any stageable kind would do; this one is named
// because the close-date corrector is the kind the product most wants a ladder
// for — a rep confirms the same shape of proposal every morning.
const countedKind = "close_date_correction"

// decidesCountedKind holds exactly the grant countedKind asks for, plus nothing
// else: the point of these tests is the counter, so the authority is the
// minimum that lets a decision through.
func decidesCountedKind() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"admin"},
		Objects:  map[string]principal.ObjectGrant{tableDeal: {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	}
}

// asRep is the context for a rep other than the env's own, so a test can prove
// one colleague's record is not another's.
func (e *stagingEnv) asRep(rep ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + rep.String(), UserID: rep,
		Permissions: decidesCountedKind(),
	})
}

// secondRep seeds another seat in the same workspace.
func (e *stagingEnv) secondRep(t *testing.T) ids.UUID {
	t.Helper()
	rep := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other')`,
		rep, "other-"+rep.String()+"@st.test"); err != nil {
		t.Fatalf("seeding a second rep: %v", err)
	}
	return rep
}

// deal seeds the record countedKind's proposals point at. The decision's
// target-visibility probe reads it, so a staging aimed at nothing would be
// refused before the counter is ever reached.
//
// The pipeline and stage come with it because a deal cannot exist without
// them, and they are seeded once per env rather than once per deal — the
// records these tests need are deals, and a pipeline per deal would be scenery.
func (e *stagingEnv) deal(t *testing.T) ids.UUID {
	t.Helper()
	e.seedPipeline(t)
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO deal (id, name, owner_id, pipeline_id, stage_id, source, captured_by)
		 VALUES ($1, 'Counted', $2, $3, $4, 'manual', $5)`,
		id, e.rep, e.pipeline, e.stage, "human:"+e.rep.String()); err != nil {
		t.Fatalf("seeding the deal a proposal is about: %v", err)
	}
	return id
}

// seedPipeline creates this env's one pipeline and stage, on first use.
func (e *stagingEnv) seedPipeline(t *testing.T) {
	t.Helper()
	if !e.pipeline.IsZero() {
		return
	}
	ctx := context.Background()
	e.pipeline, e.stage = ids.NewV7(), ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, $2, false, 0)`,
		e.pipeline, "Counted "+e.pipeline.String()); err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		 VALUES ($1, $2, 'Open', 0, 'open', 20)`, e.stage, e.pipeline); err != nil {
		t.Fatalf("seeding the stage: %v", err)
	}
}

// stageCounted stages one proposal of countedKind against a fresh deal.
//
// A new deal per staging rather than a shared one: the identity guard refuses a
// second pending proposal about the same record, and a test that wanted two
// decisions would otherwise get one staging and one silent join.
func (e *stagingEnv) stageCounted(ctx context.Context, t *testing.T) ids.ApprovalID {
	t.Helper()
	target := e.deal(t)
	id, err := e.svc.Stage(ctx, StageInput{
		Kind:           countedKind,
		ProposedChange: []byte(`{"deal_id":"` + target.String() + `","expected_close_date":"2026-12-01"}`),
		DiffHash:       target.String(),
		TargetType:     tableDeal,
		TargetID:       target,
		Summary:        "Confirm the real close date",
	})
	if err != nil {
		t.Fatalf("staging %s: %v", countedKind, err)
	}
	return id
}

// policyOf reads one rep's stored row straight from the table, so an assertion
// about what was WRITTEN never runs through the reader it is checking.
func (e *stagingEnv) policyOf(t *testing.T, rep ids.UUID) (mode string, clean, edited, rejected int) {
	t.Helper()
	err := e.owner.QueryRow(context.Background(),
		`SELECT mode, approved_clean, approved_edited, rejected
		   FROM approval_autonomy_policy WHERE user_id = $1 AND kind = $2`,
		rep, countedKind).Scan(&mode, &clean, &edited, &rejected)
	if err != nil {
		t.Fatalf("reading the stored policy for %s: %v", countedKind, err)
	}
	return mode, clean, edited, rejected
}

func TestADecisionCountsTowardTheDecidingRepsRecord(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)

	id := e.stageCounted(ctx, t)
	if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}

	mode, clean, edited, rejected := e.policyOf(t, e.rep)
	if mode != string(AutonomyManual) {
		t.Errorf("mode = %q, want %q — counting a decision must not change what the rep chose",
			mode, AutonomyManual)
	}
	if clean != 1 || edited != 0 || rejected != 0 {
		t.Errorf("clean/edited/rejected = %d/%d/%d, want 1/0/0 — an untouched approval is the one that earns promotion",
			clean, edited, rejected)
	}
}

func TestARejectionAndAnEditAreCountedApartFromACleanApproval(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)

	if _, err := e.svc.Decide(ctx, e.stageCounted(ctx, t), false, nil); err != nil {
		t.Fatalf("rejecting: %v", err)
	}
	if _, err := e.svc.Decide(ctx, e.stageCounted(ctx, t), true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}

	_, clean, edited, rejected := e.policyOf(t, e.rep)
	if clean != 1 || rejected != 1 {
		t.Errorf("clean/rejected = %d/%d, want 1/1 — the two verdicts are different evidence",
			clean, rejected)
	}
	if edited != 0 {
		t.Errorf("edited = %d, want 0 — nothing here was edited", edited)
	}
}

// A rejected decision must not be readable as agreement. This is the mutation
// case for the outcome mapping: fold rejection into approved_clean and the
// ladder offers autonomy to a rep who has refused the kind every time.
func TestRejectionsNeverEarnACleanApproval(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)

	for range 3 {
		if _, err := e.svc.Decide(ctx, e.stageCounted(ctx, t), false, nil); err != nil {
			t.Fatalf("rejecting: %v", err)
		}
	}

	_, clean, _, rejected := e.policyOf(t, e.rep)
	if clean != 0 {
		t.Errorf("approved_clean = %d after three rejections, want 0", clean)
	}
	if rejected != 3 {
		t.Errorf("rejected = %d, want 3", rejected)
	}
}

// The counter is the deciding rep's, not the installation's. Two reps deciding
// the same kind must build two records, or one colleague's refusals would hold
// down another's earned standing.
func TestOneRepsRecordIsNotAnother(t *testing.T) {
	e := setupStaging(t)
	other := e.secondRep(t)

	if _, err := e.svc.Decide(e.asRep(e.rep), e.stageCounted(e.asRep(e.rep), t), true, nil); err != nil {
		t.Fatalf("first rep approving: %v", err)
	}
	otherCtx := e.asRep(other)
	if _, err := e.svc.Decide(otherCtx, e.stageCounted(otherCtx, t), false, nil); err != nil {
		t.Fatalf("second rep rejecting: %v", err)
	}

	if _, clean, _, rejected := e.policyOf(t, e.rep); clean != 1 || rejected != 0 {
		t.Errorf("first rep clean/rejected = %d/%d, want 1/0", clean, rejected)
	}
	if _, clean, _, rejected := e.policyOf(t, other); clean != 0 || rejected != 1 {
		t.Errorf("second rep clean/rejected = %d/%d, want 0/1", clean, rejected)
	}
}

// The reader answers for the acting principal and for nobody else. A rep who
// asks about their close-date policy is told about THEIR row, whatever another
// rep has stored.
func TestAPolicyReadAnswersForTheActingRepOnly(t *testing.T) {
	e := setupStaging(t)
	other := e.secondRep(t)

	if _, err := e.svc.SetAutonomy(e.asRep(other), SetAutonomyInput{
		Kind: countedKind, Mode: AutonomyAuto,
	}); err != nil {
		t.Fatalf("second rep setting a policy: %v", err)
	}

	mine, err := e.svc.ReadAutonomy(e.asRep(e.rep), countedKind)
	if err != nil {
		t.Fatalf("reading my own policy: %v", err)
	}
	if mine.Mode != AutonomyManual {
		t.Errorf("mode = %q, want %q — another rep's choice is not mine", mine.Mode, AutonomyManual)
	}
}

// A kind nobody has decided about reads as manual rather than as absent, so a
// caller never has to tell "no row" from "ask me".
func TestAnUndecidedKindReadsAsManual(t *testing.T) {
	e := setupStaging(t)

	policy, err := e.svc.ReadAutonomy(e.asRep(e.rep), countedKind)
	if err != nil {
		t.Fatalf("reading an undecided kind: %v", err)
	}
	if policy.Mode != AutonomyManual || policy.ApprovedClean != 0 {
		t.Errorf("policy = %+v, want manual with nothing counted", policy)
	}
	if policy.Kind != countedKind {
		t.Errorf("kind = %q, want %q — the default still names what it is about",
			policy.Kind, countedKind)
	}
}

// Choosing a mode must not disturb the record the choice was made on.
func TestSettingAPolicyLeavesTheTrackRecordAlone(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)

	if _, err := e.svc.Decide(ctx, e.stageCounted(ctx, t), true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}
	if _, err := e.svc.SetAutonomy(ctx, SetAutonomyInput{
		Kind: countedKind, Mode: AutonomyVeto, Window: 4 * time.Hour,
	}); err != nil {
		t.Fatalf("setting a veto policy: %v", err)
	}

	policy, err := e.svc.ReadAutonomy(ctx, countedKind)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if policy.Mode != AutonomyVeto || policy.Window != 4*time.Hour {
		t.Errorf("mode/window = %q/%s, want veto/4h", policy.Mode, policy.Window)
	}
	if policy.ApprovedClean != 1 {
		t.Errorf("approved_clean = %d, want 1 — the history that justified the choice survives it",
			policy.ApprovedClean)
	}
	if policy.PromotedAt == nil {
		t.Error("promoted_at is unset after moving up the ladder")
	}
	if policy.DemotedAt != nil {
		t.Error("demoted_at is set after moving UP the ladder")
	}
}

// Moving back down is the demotion stamp, and it must not clear the promotion
// that came before it: both are facts about when the rep last changed standing.
func TestMovingDownTheLadderStampsADemotion(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)

	for _, mode := range []AutonomyMode{AutonomyAuto, AutonomyManual} {
		if _, err := e.svc.SetAutonomy(ctx, SetAutonomyInput{Kind: countedKind, Mode: mode}); err != nil {
			t.Fatalf("setting %s: %v", mode, err)
		}
	}

	policy, err := e.svc.ReadAutonomy(ctx, countedKind)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if policy.PromotedAt == nil {
		t.Error("promoted_at was cleared by a later demotion")
	}
	if policy.DemotedAt == nil {
		t.Error("demoted_at is unset after moving down the ladder")
	}
}

// The shapes the table refuses, refused at the service with a 422-shaped error
// so the caller is told which field was wrong rather than handed a constraint
// name. The constraints stay regardless — this guards one entry point, they
// guard every writer.
func TestAPolicyTheProductCannotKeepIsRefused(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)

	for _, tc := range []struct {
		name string
		in   SetAutonomyInput
	}{
		{
			"a veto with no window is not a chance to intervene",
			SetAutonomyInput{Kind: countedKind, Mode: AutonomyVeto},
		},
		{
			"a veto window of zero applies immediately while reading as a wait",
			SetAutonomyInput{Kind: countedKind, Mode: AutonomyVeto, Window: 0},
		},
		{
			"a window on auto is a number that looks like a promise",
			SetAutonomyInput{Kind: countedKind, Mode: AutonomyAuto, Window: time.Hour},
		},
		{
			"a mode the ladder has no rung for",
			SetAutonomyInput{Kind: countedKind, Mode: AutonomyMode("whenever")},
		},
		{
			"a kind this installation never stages",
			SetAutonomyInput{Kind: "nonexistent_kind", Mode: AutonomyManual},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.SetAutonomy(ctx, tc.in)
			var invalid *InvalidAutonomyError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want an InvalidAutonomyError", err)
			}
			if e.count(t, `SELECT count(*) FROM approval_autonomy_policy WHERE user_id = $1`, e.rep) != 0 {
				t.Error("a refused policy still wrote a row")
			}
		})
	}
}

// A veto window that survives the round trip: the column is an interval and the
// field is a Duration, so a scale error here would read as a policy that waits
// hours instead of seconds without anything failing.
func TestAVetoWindowRoundTripsThroughTheInterval(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asRep(e.rep)

	const window = 90 * time.Minute
	if _, err := e.svc.SetAutonomy(ctx, SetAutonomyInput{
		Kind: countedKind, Mode: AutonomyVeto, Window: window,
	}); err != nil {
		t.Fatalf("setting a veto policy: %v", err)
	}

	policy, err := e.svc.ReadAutonomy(ctx, countedKind)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if policy.Window != window {
		t.Errorf("window = %s, want %s", policy.Window, window)
	}
}

// Every kind, not every stored row: a rep who has decided nothing must still be
// offered every choice, or the surface renders as "nothing to set up".
func TestTheListOffersEveryStageableKind(t *testing.T) {
	e := setupStaging(t)

	policies, err := e.svc.ListAutonomy(e.asRep(e.rep))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(policies) != len(StageableKinds()) {
		t.Fatalf("listed %d kinds, want %d — a kind with no row is still a choice",
			len(policies), len(StageableKinds()))
	}
	for _, policy := range policies {
		if policy.Mode != AutonomyManual {
			t.Errorf("%s = %q, want manual on a fresh account", policy.Kind, policy.Mode)
		}
	}
}

// The system principal decides nothing, so it counts nothing. Without this the
// expiry sweep and any background decider would build a track record against a
// user id no person answers for.
func TestAPrincipalWithNoHumanBehindItCannotSetAPolicy(t *testing.T) {
	e := setupStaging(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:test",
	})

	if _, err := e.svc.SetAutonomy(ctx, SetAutonomyInput{
		Kind: countedKind, Mode: AutonomyAuto,
	}); err == nil {
		t.Fatal("a system principal set an autonomy policy")
	}
	if _, err := e.svc.ListAutonomy(ctx); err == nil {
		t.Fatal("a system principal read the ladder")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// The track record a rep builds by deciding, against a real database.
//
// Everything worth proving here is what the schema and the transaction hold:
// that a decision and the counter it earns commit together, that the three
// outcomes land in three different columns, and that one rep's record is not
// another's. None of it can be shown without Postgres.

import (
	"bytes"
	"context"
	"testing"

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

// dealSeeder makes the deals countedKind's proposals point at, holding the one
// pipeline and stage they all sit in.
//
// It is a type of its own rather than two more fields on the shared stagingEnv,
// which five other suites use and none of the others needs: a fixture that
// grows a field per suite ends up carrying everyone's scenery.
type dealSeeder struct {
	env      *stagingEnv
	pipeline ids.UUID
	stage    ids.UUID
}

// newDealSeeder creates the pipeline and stage every deal below shares. One
// pipeline per suite rather than per deal: the records these tests are about
// are deals, and a pipeline each would be scenery.
func newDealSeeder(t *testing.T, e *stagingEnv) *dealSeeder {
	t.Helper()
	ctx := context.Background()
	s := &dealSeeder{env: e, pipeline: ids.NewV7(), stage: ids.NewV7()}
	// The name carries the id because pipeline names are unique installation-wide
	// and this package's suites share one database.
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, $2, false, 0)`,
		s.pipeline, "Counted "+s.pipeline.String()); err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		 VALUES ($1, $2, 'Open', 0, 'open', 20)`, s.stage, s.pipeline); err != nil {
		t.Fatalf("seeding the stage: %v", err)
	}
	return s
}

// deal seeds one record for a proposal to point at. The decision's
// target-visibility probe reads it, so a staging aimed at nothing would be
// refused before the counter is ever reached.
func (s *dealSeeder) deal(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := s.env.owner.Exec(context.Background(),
		`INSERT INTO deal (id, name, owner_id, pipeline_id, stage_id, source, captured_by)
		 VALUES ($1, 'Counted', $2, $3, $4, 'manual', $5)`,
		id, s.env.rep, s.pipeline, s.stage, "human:"+s.env.rep.String()); err != nil {
		t.Fatalf("seeding the deal a proposal is about: %v", err)
	}
	return id
}

// stageCounted stages one proposal of countedKind against a fresh deal.
//
// A new deal per staging rather than a shared one: the identity guard refuses a
// second pending proposal about the same record, and a test that wanted two
// decisions would otherwise get one staging and one silent join.
func (s *dealSeeder) stageCounted(ctx context.Context, t *testing.T) ids.ApprovalID {
	t.Helper()
	target := s.deal(t)
	id, err := s.env.svc.Stage(ctx, StageInput{
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
	seed := newDealSeeder(t, e)
	ctx := e.asRep(e.rep)

	id := seed.stageCounted(ctx, t)
	if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("approving: %v", err)
	}

	mode, clean, edited, rejected := e.policyOf(t, e.rep)
	// The row the counter creates carries the column's default, and nothing in
	// this package writes it. A decision that arrived carrying a mode would be
	// the counter deciding what a rep had chosen.
	if mode != "manual" {
		t.Errorf("mode = %q, want manual — counting a decision must not set a policy", mode)
	}
	if clean != 1 || edited != 0 || rejected != 0 {
		t.Errorf("clean/edited/rejected = %d/%d/%d, want 1/0/0 — an untouched approval is the one that earns promotion",
			clean, edited, rejected)
	}
}

func TestARejectionAndAnEditAreCountedApartFromACleanApproval(t *testing.T) {
	e := setupStaging(t)
	seed := newDealSeeder(t, e)
	ctx := e.asRep(e.rep)

	if _, err := e.svc.Decide(ctx, seed.stageCounted(ctx, t), false, nil); err != nil {
		t.Fatalf("rejecting: %v", err)
	}
	if _, err := e.svc.Decide(ctx, seed.stageCounted(ctx, t), true, nil); err != nil {
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

// An approval the rep rewrote counts as an edit, not as agreement with what was
// proposed. This is the case the ladder exists to tell apart: a kind whose
// payload is corrected every time is the kind the software must keep asking
// about, and counting those as clean approvals would promote it fastest.
func TestAnEditedApprovalIsNotACleanOne(t *testing.T) {
	e := setupStaging(t)
	seed := newDealSeeder(t, e)
	ctx := e.asRep(e.rep)

	id := seed.stageCounted(ctx, t)
	staged, err := e.svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("reading the staged proposal: %v", err)
	}
	// The same deal, a different date: an edit may correct the proposal but
	// never re-aim it, so the deal_id has to survive verbatim.
	edited := bytes.Replace(staged.ProposedChange,
		[]byte(`"2026-12-01"`), []byte(`"2027-03-15"`), 1)
	if bytes.Equal(edited, staged.ProposedChange) {
		t.Fatal("the edit changed nothing, so this proves nothing about editing")
	}
	if _, err := e.svc.DecideEdited(ctx, id, edited); err != nil {
		t.Fatalf("approving with an edit: %v", err)
	}

	_, clean, editedCount, rejected := e.policyOf(t, e.rep)
	if editedCount != 1 {
		t.Errorf("approved_edited = %d, want 1 — the rep rewrote the payload", editedCount)
	}
	if clean != 0 {
		t.Errorf("approved_clean = %d, want 0 — an edit is a correction, not agreement", clean)
	}
	if rejected != 0 {
		t.Errorf("rejected = %d, want 0", rejected)
	}
}

// A rejected decision must not be readable as agreement. This is the mutation
// case for the outcome mapping: fold rejection into approved_clean and the
// ladder offers autonomy to a rep who has refused the kind every time.
func TestRejectionsNeverEarnACleanApproval(t *testing.T) {
	e := setupStaging(t)
	seed := newDealSeeder(t, e)
	ctx := e.asRep(e.rep)

	for range 3 {
		if _, err := e.svc.Decide(ctx, seed.stageCounted(ctx, t), false, nil); err != nil {
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
	seed := newDealSeeder(t, e)
	other := e.secondRep(t)

	if _, err := e.svc.Decide(e.asRep(e.rep), seed.stageCounted(e.asRep(e.rep), t), true, nil); err != nil {
		t.Fatalf("first rep approving: %v", err)
	}
	otherCtx := e.asRep(other)
	if _, err := e.svc.Decide(otherCtx, seed.stageCounted(otherCtx, t), false, nil); err != nil {
		t.Fatalf("second rep rejecting: %v", err)
	}

	if _, clean, _, rejected := e.policyOf(t, e.rep); clean != 1 || rejected != 0 {
		t.Errorf("first rep clean/rejected = %d/%d, want 1/0", clean, rejected)
	}
	if _, clean, _, rejected := e.policyOf(t, other); clean != 0 || rejected != 1 {
		t.Errorf("second rep clean/rejected = %d/%d, want 0/1", clean, rejected)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Undoing a filing under a project.
//
// The class a project filing stamps is what shields correspondence from
// erasure, so the cases that matter are the ones where the undo must NOT take
// it away. Every filing here goes through the real relink writer and every
// qualifying deal through the real winning transition: a fixture that inserted
// the evidence by hand would prove the columns exist and nothing about what the
// undo reads.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const undoReason = "filed under the wrong project by an assistant"

// filedUnderAProject is an email filed under a project through the relink
// writer, so the class, the evidence and the link are what production wrote.
func filedUnderAProject(t *testing.T, e *Env) projectStampFixture {
	t.Helper()
	f := seedProjectStampFixture(t, e)
	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.ActivityID{UUID: f.email},
		activities.RelinkActivityInput{EntityType: "project", EntityID: f.project}); err != nil {
		t.Fatalf("filing the email under its project: %v", err)
	}
	return f
}

// theFilingStands says the undo changed nothing: class, evidence and link are
// all still there.
func theFilingStands(t *testing.T, e *Env, f projectStampFixture) {
	t.Helper()
	got := readProjectStamp(t, e, f.email)
	if got.class == nil || got.evidence != 1 {
		t.Errorf("the refused undo moved the record: class=%v project evidence=%d", got.class, got.evidence)
	}
	if projectLinks(t, e, f.email, f.project) != 1 {
		t.Error("the refused undo unfiled the activity")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND evidence->>'cause' = 'project_filing_undone'`, f.email); n != 0 {
		t.Errorf("a refused undo left %d audit entries, want none", n)
	}
}

// winDeal advances the fixture's deal to won through the real transition, which
// is what stamps the correspondence filed against it at that moment.
func winDeal(t *testing.T, e *Env, f stampFixture) {
	t.Helper()
	if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.DealID{UUID: f.deal}, wonInput(f.wonStage)); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}
}

func linkToDeal(t *testing.T, e *Env, activity, deal ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, activity, deal)
}

func refusalCode(t *testing.T, err error) string {
	t.Helper()
	var refused *activities.ProjectFilingRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("undo → %v, want a refusal the caller can act on", err)
	}
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a refusal of the record's state must answer 409, got %v", err)
	}
	return string(refused.Code)
}

// The undo unfiles the activity and withdraws the class in one transaction with
// its audit entry and event, and the decision reads back on the activity.
func TestUndoingAProjectFilingWithdrawsTheClassAndRecordsTheDecision(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	id := ids.ActivityID{UUID: f.email}

	state, err := e.Activities.GetProjectFiling(e.Admin(), id)
	if err != nil {
		t.Fatalf("reading the filing: %v", err)
	}
	if !state.Filed || !state.Undoable || state.Refusal != nil || len(state.Projects) != 1 || state.Projects[0].Name != "ERP rollout" {
		t.Fatalf("a freshly filed activity reads as %+v, want filed, undoable and naming its project", state)
	}

	after, err := e.Activities.UndoProjectFiling(e.Admin(), id, "  "+undoReason+"  ")
	if err != nil {
		t.Fatalf("undoing the filing: %v", err)
	}
	if after.Filed || after.Undoable || len(after.Projects) != 0 || len(after.Undone) != 1 {
		t.Fatalf("the answer after the undo is %+v, want unfiled with the decision on record", after)
	}
	if got := after.Undone[0]; got.Reason != undoReason || got.ByName != "Rep" || len(got.Projects) != 1 || got.Projects[0] != "ERP rollout" {
		t.Errorf("the decision on record is %+v, want the trimmed reason, the member's name and the project", got)
	}

	stamp := readProjectStamp(t, e, f.email)
	if stamp.class != nil || stamp.stampAt != nil || stamp.evidence != 0 {
		t.Errorf("after the undo: class=%v at=%v evidence=%d, want all withdrawn", stamp.class, stamp.stampAt, stamp.evidence)
	}
	if projectLinks(t, e, f.email, f.project) != 0 {
		t.Error("the activity is still filed under the project")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'
		AND evidence->>'cause' = 'project_filing_undone' AND evidence->>'reason' = $2`, f.email, undoReason); n != 1 {
		t.Errorf("audit entries for the undo = %d, want 1 carrying the reason", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'activity.updated'
		AND envelope->'entity'->>'id' = $1::text AND envelope::text LIKE '%project_filing_undone%'`, f.email); n != 1 {
		t.Errorf("activity.updated events for the undo = %d, want 1", n)
	}

	_, err = e.Activities.UndoProjectFiling(e.Admin(), id, undoReason)
	if got := refusalCode(t, err); got != "not_filed" {
		t.Errorf("a second undo refuses with %q, want not_filed", got)
	}
}

// Another qualifying basis keeps the class, so the project filing alone cannot
// be undone while a won deal still holds the correspondence.
func TestAFilingIsNotUndoneWhileAWonDealStillQualifiesTheActivity(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	deal := seedStampFixture(t, e)
	linkToDeal(t, e, f.email, deal.deal)
	winDeal(t, e, deal)

	state, err := e.Activities.GetProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email})
	if err != nil || state.Undoable || state.Refusal == nil || string(state.Refusal.Code) != "other_basis_remains" {
		t.Fatalf("the filing reads as %+v (err %v), want refused for another basis", state, err)
	}
	_, err = e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
	if got := refusalCode(t, err); got != "other_basis_remains" {
		t.Errorf("refusal = %q, want other_basis_remains", got)
	}
	theFilingStands(t, e, f)
}

// A deal won before the activity was linked to it left no evidence row behind,
// but the correspondence still qualifies through it.
func TestAFilingIsNotUndoneWhileALinkedDealStillQualifiesWithoutEvidence(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	deal := seedStampFixture(t, e)
	winDeal(t, e, deal)
	linkToDeal(t, e, f.email, deal.deal)

	_, err := e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
	if got := refusalCode(t, err); got != "qualifying_deal" {
		t.Errorf("refusal = %q, want qualifying_deal", got)
	}
	theFilingStands(t, e, f)
}

// A hold that has started never shortens: a restricted activity keeps its class
// and its project, whoever asks.
func TestAFilingIsNotUndoneOnceAStatutoryHoldHasStarted(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	reason, err := privacy.ParseStatedReason("controller pin")
	if err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(e.DB()).PinToFloor(controllerCtx(e, principal.ObjectGrant{Read: true, Update: true}), f.email, reason); err != nil {
		t.Fatalf("pinning the activity: %v", err)
	}

	_, err = e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
	if got := refusalCode(t, err); got != "restricted" {
		t.Errorf("refusal = %q, want restricted", got)
	}
	if projectLinks(t, e, f.email, f.project) != 1 || readProjectStamp(t, e, f.email).class == nil {
		t.Error("a restricted activity lost its project or its class")
	}
}

// Who decides, and on what words: no reason, no grant, or an agent changes
// nothing — even an agent holding every permission, acting for the member.
func TestOnlyANamedMemberWithAReasonUndoesAFiling(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	id := ids.ActivityID{UUID: f.email}
	readOnly := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
		RowScope: principal.RowScopeAll,
	})

	for name, tc := range map[string]struct {
		try    func() error
		denied bool
	}{
		"a blank reason": {func() error { _, err := e.Activities.UndoProjectFiling(e.Admin(), id, " \n "); return err }, false},
		"a reason past the bound": {func() error {
			_, err := e.Activities.UndoProjectFiling(e.Admin(), id, strings.Repeat("x", 2001))
			return err
		}, false},
		"a reason padded past the bound with whitespace": {func() error {
			_, err := e.Activities.UndoProjectFiling(e.Admin(), id, "  "+strings.Repeat("x", 2000))
			return err
		}, false},
		"a member without activity.update": {func() error { _, err := e.Activities.UndoProjectFiling(readOnly, id, undoReason); return err }, true},
		"an agent acting for an admin": {func() error {
			_, err := e.Activities.UndoProjectFiling(relinkAgentCtx(e, e.AgentPassport), id, undoReason)
			return err
		}, true},
	} {
		err := tc.try()
		var detailed *httperr.DetailedError
		switch {
		case err == nil:
			t.Errorf("%s undid the filing", name)
		case tc.denied && !errors.Is(err, apperrors.ErrPermissionDenied):
			t.Errorf("%s → %v, want a permission denial", name, err)
		case !tc.denied && (!errors.As(err, &detailed) || len(detailed.Fields) != 1 || detailed.Fields[0].Field != "reason"):
			t.Errorf("%s → %v, want a validation refusal naming the reason field", name, err)
		}
	}
	theFilingStands(t, e, f)
}

// The shield is what an erasure reads. An activity the project filing alone
// held is ordinary again once undone; one a won deal ALSO holds is still held.
func TestUndoingAFilingMovesTheErasureShieldOnlyWhereTheFilingWasAlone(t *testing.T) {
	alone := Setup(t)
	f := seedProjectErasureFixture(t, alone)
	if _, err := alone.Activities.RelinkActivity(alone.Admin(), ids.ActivityID{UUID: f.email},
		activities.RelinkActivityInput{EntityType: "project", EntityID: f.project}); err != nil {
		t.Fatal(err)
	}
	if _, err := alone.Activities.UndoProjectFiling(alone.Admin(), ids.ActivityID{UUID: f.email}, undoReason); err != nil {
		t.Fatalf("undoing the sole basis: %v", err)
	}
	if err := privacy.NewEraser(alone.DB()).EraseContact(alone.Admin(), f.contact, "test"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	if got := readHeldState(t, alone, f.email); got.restrictedAt != nil {
		t.Error("an activity whose only basis was undone is still held after the erasure")
	}
}

func TestACorrespondenceTheDealStillQualifiesStaysHeldWhenItsFilingIsRefused(t *testing.T) {
	e := Setup(t)
	f := seedProjectErasureFixture(t, e)
	id := ids.ActivityID{UUID: f.email}
	if _, err := e.Activities.RelinkActivity(e.Admin(), id,
		activities.RelinkActivityInput{EntityType: "project", EntityID: f.project}); err != nil {
		t.Fatal(err)
	}
	deal := seedStampFixture(t, e)
	linkToDeal(t, e, f.email, deal.deal)
	winDeal(t, e, deal)

	if _, err := e.Activities.UndoProjectFiling(e.Admin(), id, undoReason); err == nil {
		t.Fatal("the filing was undone although a won deal still qualifies the correspondence")
	}
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), f.contact, "test"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	got := readHeldState(t, e, f.email)
	if got.restrictedAt == nil || got.body == nil {
		t.Errorf("correspondence a won deal still qualifies lost its shield: %+v", got)
	}
}

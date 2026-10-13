// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What stands between an undo and a record that must stay retained: a legal
// hold, a deal that still qualifies the correspondence, a project the member
// cannot see, an erasure already in motion, and a deal winning at the very
// moment the filing is withdrawn.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// A legal hold on the project an activity was filed under outranks the undo: the
// writer refuses it by name and the trigger refuses a direct delete of the proof.
func TestAFilingUnderAHeldProjectIsNotUndone(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := filedUnderAProject(t, e)
	e.WsExec(t, `UPDATE project SET legal_hold = true WHERE id = $1`, f.project)

	state, err := e.Activities.GetProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email})
	if err != nil || state.Undoable || state.Refusal == nil || string(state.Refusal.Code) != "legal_hold" {
		t.Fatalf("the filing reads as %+v (err %v), want refused for the hold", state, err)
	}
	_, err = e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
	if got := refusalCode(t, err); got != "legal_hold" {
		t.Errorf("refusal = %q, want legal_hold", got)
	}
	theFilingStands(t, e, f)

	// The writer is not the only door: a declared delete is refused by the data layer.
	declared := step{declareUndo, []any{f.email.String()}}
	if got := refusedBy(t, owner, declared, step{deleteFilingProof, []any{f.email}}); got != "activity_retention_evidence_frozen" {
		t.Errorf("a declared delete of evidence under a held project was refused by %q", got)
	}
	// Even once the link is gone, the evidence still names the held project.
	if got := refusedBy(t, owner, declared, step{unlinkProject, []any{f.email}}, step{deleteFilingProof, []any{f.email}}); got != "activity_retention_evidence_frozen" {
		t.Errorf("a declared delete after unlinking under a held project was refused by %q", got)
	}
}

// The triggers restate retentionscope's predicates in SQL a trigger can run; for
// each state of the activity's links, the database refuses the clear exactly when
// the Go fragments say something still keeps the activity.
func TestTheUndoTriggersAgreeWithTheGoPredicates(t *testing.T) {
	for name, tc := range map[string]struct {
		arrange func(t *testing.T, e *Env, f projectStampFixture)
	}{
		"nothing else keeps it": {func(*testing.T, *Env, projectStampFixture) {}},
		"a won deal linked after the win": {func(t *testing.T, e *Env, f projectStampFixture) {
			deal := seedStampFixture(t, e)
			winDeal(t, e, deal)
			linkToDeal(t, e, f.email, deal.deal)
		}},
		"a held company": {func(t *testing.T, e *Env, f projectStampFixture) {
			company := e.SeedCompany(t, "Held GmbH", nil)
			e.WsExec(t, `UPDATE company SET legal_hold = true WHERE id = $1`, company)
			e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, f.email, company)
		}},
		"a held deal that has not qualified": {func(t *testing.T, e *Env, f projectStampFixture) {
			deal := seedStampFixture(t, e)
			e.WsExec(t, `UPDATE deal SET legal_hold = true WHERE id = $1`, deal.deal)
			linkToDeal(t, e, f.email, deal.deal)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			owner := OwnerConn(t)
			f := filedUnderAProject(t, e)
			tc.arrange(t, e, f)

			var kept bool
			expr := "'" + f.email.String() + "'::uuid"
			if err := owner.QueryRow(context.Background(), `SELECT `+retentionscope.QualifyingDealLink(expr)+` OR `+
				retentionscope.HeldThroughAnyLink(expr)).Scan(&kept); err != nil {
				t.Fatal(err)
			}
			got := refusedBy(t, owner, step{declareUndo, []any{f.email.String()}}, step{unlinkProject, []any{f.email}},
				step{deleteFilingProof, []any{f.email}}, step{clearTheClass, []any{f.email}})
			if refused := got != ""; refused != kept {
				t.Errorf("the Go predicates say kept=%v, the triggers refused=%v (%q)", kept, refused, got)
			}
		})
	}
}

// A member who can see the activity but not the project learns neither its name
// nor the words of a decision about it, and the write agrees with the read.
func TestAProjectTheMemberCannotSeeIsUnnamedAndStillHoldsTheActivity(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	contact := e.SeedContact(t, "Visible Contact", &e.Rep1)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, f.email, contact)
	id := ids.ActivityID{UUID: f.email}
	noProjects := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Update: true}, "contact": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})

	state, err := e.Activities.GetProjectFiling(noProjects, id)
	if err != nil {
		t.Fatalf("reading the filing: %v", err)
	}
	if len(state.Projects) != 1 || state.Projects[0].Name != "" || state.Projects[0].Hidden == nil || !*state.Projects[0].Hidden {
		t.Errorf("the project reads as %+v, want it unnamed and hidden", state.Projects)
	}
	if state.Undoable || state.Refusal == nil || string(state.Refusal.Code) != "hidden_project" {
		t.Errorf("the verdict is %+v, want refused because a project the member cannot see still holds it", state)
	}
	_, err = e.Activities.UndoProjectFiling(noProjects, id, undoReason)
	if got := refusalCode(t, err); got != "hidden_project" {
		t.Errorf("the write refused with %q, want the read's own answer", got)
	}
	theFilingStands(t, e, f)

	// An admin who can see it undoes it; the member then sees only that it happened.
	if _, err := e.Activities.UndoProjectFiling(e.Admin(), id, undoReason); err != nil {
		t.Fatalf("the admin's undo: %v", err)
	}
	after, err := e.Activities.GetProjectFiling(noProjects, id)
	if err != nil || len(after.Undone) != 1 {
		t.Fatalf("the member reads %+v (err %v), want one decision on record", after, err)
	}
	if got := after.Undone[0]; got.Redacted == nil || !*got.Redacted || got.Reason != "" || got.ByName != "" || len(got.Projects) != 0 {
		t.Errorf("the decision reads as %+v, want only its moment", got)
	}
}

// A seat holding only a project grant is refused the read, as for any activity read.
func TestTheFilingReadNeedsTheActivityReadGrant(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	noActivities := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	if _, err := e.Activities.GetProjectFiling(noActivities, ids.ActivityID{UUID: f.email}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("reading the filing without the activity grant → %v, want a permission denial", err)
	}
}

// A decider with no display name is told what to fix, not refused as a stranger.
func TestAnUnnamedMemberIsToldToSetADisplayName(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	e.WsExec(t, `UPDATE app_user SET display_name = '' WHERE id = $1`, e.AdminUser)

	_, err := e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
	var unnamed *activities.DeciderUnnamedError
	if !errors.As(err, &unnamed) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undo → %v, want the display-name refusal as a conflict", err)
	}
	theFilingStands(t, e, f)
}

// Whether a credential's own release of a project relink can be taken back is a
// question about the activity's state.
func TestAProjectRelinkIsReleasableOnlyWhileTheUndoCouldTakeItBack(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	clean, held, erasing, restricted := f.mine[0], f.mine[1], ids.NewV7(), ids.NewV7()
	contact := e.SeedContact(t, "Erasure Subject", &e.Rep1)
	for _, id := range []ids.UUID{erasing, restricted} {
		e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'x', 'body', now(), 'manual', 'human:x')`, id)
	}
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, erasing, contact)
	e.WsExec(t, `INSERT INTO data_subject_request (kind, status, subject_ref, due_at, contact_id)
		VALUES ('erasure', 'open', $1::text, now() + interval '30 days', $2)`, contact.String(), contact)
	company := e.SeedCompany(t, "Held GmbH", nil)
	e.WsExec(t, `UPDATE company SET legal_hold = true WHERE id = $1`, company)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, held, company)
	e.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, project_id, project_name)
		VALUES ($1, 'project_linked', now(), $2, 'ERP rollout')`, restricted, f.project)
	e.WsExec(t, `UPDATE activity SET archived_at = now(), restricted_at = now(), restricted_until = now() + interval '6 years',
		restricted_reason = 'commercial_correspondence', retention_class = 'commercial_correspondence', retention_class_at = now()
		WHERE id = $1`, restricted)

	// Kept by another basis, and by a deal that qualifies it with no evidence row:
	// the undo refuses both, so a credential's release must not be offered them.
	stamped := seedStampFixture(t, e)
	winDeal(t, e, stamped)
	late := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'late', 'body', now(), 'manual', 'human:x')`, late)
	linkToDeal(t, e, late, stamped.deal)
	archived := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, archived_at)
		VALUES ($1, 'email', 'archived', 'body', now(), 'manual', 'human:x', now())`, archived)

	for name, tc := range map[string]struct {
		activities []ids.UUID
		want       bool
	}{
		"an activity kept by another basis":     {[]ids.UUID{stamped.email}, false},
		"an activity a won deal qualifies":      {[]ids.UUID{late}, false},
		"an archived activity":                  {[]ids.UUID{archived}, false},
		"an ordinary activity":                  {[]ids.UUID{clean}, true},
		"an activity held through a link":       {[]ids.UUID{held}, false},
		"an activity under an open erasure":     {[]ids.UUID{erasing}, false},
		"an activity already restricted":        {[]ids.UUID{restricted}, false},
		"one bad activity in a clean set":       {[]ids.UUID{clean, erasing}, false},
		"an id that names no activity":          {[]ids.UUID{clean, ids.NewV7()}, false},
		"no activities":                         {nil, false},
		"the same activity twice, which is one": {[]ids.UUID{clean, clean}, true},
	} {
		got, err := staysUndoable(e, tc.activities, f.project)
		if err != nil || got != tc.want {
			t.Errorf("%s: undoable = %v (err %v), want %v", name, got, err, tc.want)
		}
	}
	e.WsExec(t, `UPDATE project SET legal_hold = true WHERE id = $1`, f.project)
	if got, err := staysUndoable(e, []ids.UUID{clean}, f.project); err != nil || got {
		t.Errorf("a filing under a held project: undoable = %v (err %v), want false", got, err)
	}
}

// An undo and a deal's win racing: the win's stamp reads the class as still set
// and writes only its evidence, so an undo that cleared the class before that
// evidence landed would leave evidence on an unclassed row. The undo takes the
// deal's lock first, so it waits for the win and then reads its evidence.
func TestAnUndoWaitsForADealWinInFlightAndThenRefuses(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	ctx := context.Background()
	f := filedUnderAProject(t, e)
	deal := seedStampFixture(t, e)
	linkToDeal(t, e, f.email, deal.deal)

	winning, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := winning.Exec(ctx, `SELECT 1 FROM deal WHERE id = $1 FOR UPDATE`, deal.deal); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
		finished <- err
	}()
	waitUntilBlockedBy(t, winning)

	// What the win's own transaction writes, through the real stamp writer.
	if err := activities.StampCorrespondenceForDeal(ctx, winning, ids.DealID{UUID: deal.deal}, "deal_won"); err != nil {
		t.Fatalf("stamping as the win does: %v", err)
	}
	if err := winning.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	err = <-finished
	if got := refusalCode(t, err); got != "other_basis_remains" {
		t.Errorf("the undo that waited for the win refused with %q, want other_basis_remains", got)
	}
	if got := readProjectStamp(t, e, f.email); got.class == nil {
		t.Error("the class was withdrawn although the win's evidence landed")
	}
}

// waitUntilBlockedBy waits until some session is blocked by the transaction
// held on holder's connection, asked of the server through pg_blocking_pids on
// a connection of its own, paced by a ticker and bounded by a deadline.
func waitUntilBlockedBy(t *testing.T, holder interface {
	QueryRow(context.Context, string, ...any) pgx.Row
},
) {
	t.Helper()
	var holderPID int
	if err := holder.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	observer := OwnerConn(t)
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked int
		// A fresh reading each pass: the statistics view is a per-transaction snapshot.
		if _, err := observer.Exec(context.Background(), `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		if err := observer.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))`, holderPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("the undo never queued behind the writer's row lock, so it does not serialize with it")
		case <-tick.C:
		}
	}
}

// A credential is not offered, and is refused, the release of a project relink
// over an activity an erasure is about to restrict: that filing could never be
// undone.
func TestACredentialDoesNotReleaseAProjectRelinkOverAnActivityUnderErasure(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	relinkLender(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	agent := relinkAgentCtx(e, e.SeedPassport(t, owner, "relink erasure"))
	var contact ids.UUID
	if err := owner.QueryRow(context.Background(),
		`SELECT contact_id FROM activity_link WHERE activity_id = $1 AND contact_id IS NOT NULL`, f.mine[0]).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `INSERT INTO data_subject_request (kind, status, subject_ref, due_at, contact_id)
		VALUES ('erasure', 'open', $1::text, now() + interval '30 days', $2)`, contact.String(), contact)

	_, err := registry.Invoke(agent, "relink_activities", json.RawMessage(
		`{"activity_ids":["`+f.mine[0].String()+`"],"entity_type":"project","entity_id":"`+f.project.String()+`"}`,
	))
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("relink_activities → %v, want a staged approval", err)
	}
	if staged.ReleasableByCaller {
		t.Error("the staged answer offers a release of a filing an erasure would make permanent")
	}
	if _, err := registry.Invoke(agent, "decide_approval", json.RawMessage(
		`{"staged_action_id":"`+staged.ApprovalID.String()+`","decision":"approve"}`,
	)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the credential's own release → %v, want a permission denial", err)
	}
	if projectLinks(t, e, f.mine[0], f.project) != 0 {
		t.Error("the refused release filed the activity")
	}
}

// staysUndoable asks the guard through the transaction a decision would already
// hold, as the approval engine does.
func staysUndoable(e *Env, named []ids.UUID, project ids.UUID) (bool, error) {
	var stays bool
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		stays, err = activities.FilingsStayUndoable(e.Admin(), tx, named, project)
		return err
	})
	return stays, err
}

// A legal hold written while the undo is being judged cannot slip between its
// reading of the holds and its clearing of the class: the undo holds the project
// FOR SHARE, so it waits for the hold and then refuses.
func TestAnUndoWaitsForALegalHoldBeingPlacedAndThenRefuses(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	ctx := context.Background()
	f := filedUnderAProject(t, e)

	placing, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := placing.Exec(ctx, `UPDATE project SET legal_hold = true WHERE id = $1`, f.project); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
		finished <- err
	}()
	waitUntilBlockedBy(t, placing)
	if err := placing.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if got := refusalCode(t, <-finished); got != "legal_hold" {
		t.Errorf("the undo that waited for the hold refused with %q, want legal_hold", got)
	}
	theFilingStands(t, e, f)
}

// An open erasure request covering a contact on the activity keeps the class, on
// the write and on the read alike.
func TestAFilingUnderAnOpenErasureRequestIsNotUndone(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	contact := e.SeedContact(t, "Erasure Subject", &e.Rep1)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, f.email, contact)
	e.WsExec(t, `INSERT INTO data_subject_request (kind, status, subject_ref, due_at, contact_id)
		VALUES ('erasure', 'in_progress', $1::text, now() + interval '30 days', $2)`, contact.String(), contact)
	id := ids.ActivityID{UUID: f.email}

	state, err := e.Activities.GetProjectFiling(e.Admin(), id)
	if err != nil || state.Undoable || state.Refusal == nil || string(state.Refusal.Code) != "erasure_pending" {
		t.Fatalf("the filing reads as %+v (err %v), want refused for the erasure request", state, err)
	}
	_, err = e.Activities.UndoProjectFiling(e.Admin(), id, undoReason)
	if got := refusalCode(t, err); got != "erasure_pending" {
		t.Errorf("refusal = %q, want erasure_pending", got)
	}
	theFilingStands(t, e, f)
}

// A caller who may not read deals is told that something else keeps the activity,
// not that a deal does.
func TestACallerWithoutTheDealGrantIsNotToldWhichDealKeepsTheActivity(t *testing.T) {
	e := Setup(t)
	f := filedUnderAProject(t, e)
	deal := seedStampFixture(t, e)
	winDeal(t, e, deal)
	linkToDeal(t, e, f.email, deal.deal)
	e.WsExec(t, `DELETE FROM activity_retention_evidence WHERE activity_id = $1 AND basis <> 'project_linked'`, f.email)
	noDeals := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Update: true}, "project": {Read: true}, "contact": {Read: true}, "company": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})

	_, err := e.Activities.UndoProjectFiling(noDeals, ids.ActivityID{UUID: f.email}, undoReason)
	if got := refusalCode(t, err); got != "other_basis_remains" {
		t.Errorf("a seat without the deal grant was refused with %q, want the generic other_basis_remains", got)
	}
	_, err = e.Activities.UndoProjectFiling(e.Admin(), ids.ActivityID{UUID: f.email}, undoReason)
	if got := refusalCode(t, err); got != "qualifying_deal" {
		t.Errorf("a seat with the deal grant was refused with %q, want qualifying_deal", got)
	}
}

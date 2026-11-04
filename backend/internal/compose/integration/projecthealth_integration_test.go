// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The project health job over real rows. It writes a reading with a reason as
// the AI, tells the owner and keeps a user's newer reading. An undo puts back
// what stood before it.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// wallNow is the instant these tests date their rows from. It is wall time and
// not clocktest.Now. The harness's projects store stamps a user's reading with
// the wall clock and refuses one dated after it. A fixture moved into the
// future would be refused rather than tested.
func wallNow() time.Time { return time.Now().UTC() }

// quietDeliveringProject is a project in delivery, owned by owner, whose last
// filed activity is 45 days old: past the quiet rule's window.
func quietDeliveringProject(t *testing.T, e *Env, name string, owner *ids.UUID, now time.Time) ids.ProjectID {
	t.Helper()
	admin := e.Admin()
	company := e.SeedCompany(t, name+" client", nil)
	p := seedProject(admin, t, e, name, company, owner)
	advanceProject(admin, t, e, p.ID, projects.PhaseDelivering)
	fileActivity(admin, t, e, "meeting", now.AddDate(0, 0, -45), &p.ID)
	return p.ID
}

func healthPass(t *testing.T, e *Env, now time.Time) compose.ProjectHealthPass {
	t.Helper()
	pass, err := compose.NewProjectHealthScan(e.Pool, func() time.Time { return now }).RunWorkspace(t.Context(), e.WS)
	if err != nil {
		t.Fatalf("project health pass: %v", err)
	}
	return pass
}

func currentHealth(t *testing.T, e *Env, id ids.ProjectID) *crmcontracts.ProjectHealthAssessment {
	t.Helper()
	var current *crmcontracts.ProjectHealthAssessment
	admin := e.Admin()
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		var err error
		current, err = e.Projects.CurrentHealthTx(admin, tx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the current health: %v", err)
	}
	return current
}

func healthHistory(t *testing.T, e *Env, id ids.ProjectID) []crmcontracts.ProjectHealthAssessment {
	t.Helper()
	rows, _, err := e.Projects.ListHealth(e.Admin(), id, nil, nil)
	if err != nil {
		t.Fatalf("listing the health history: %v", err)
	}
	return rows
}

func healthNotices(t *testing.T, e *Env, project ids.ProjectID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'project_health_ai' AND target_id = $1`, project)
}

func TestTheHealthJobMarksAQuietProjectAtRiskAndTellsTheOwner(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)

	if pass := healthPass(t, e, now); pass.Considered != 1 || pass.Written != 1 {
		t.Fatalf("first pass = %+v, want one project considered and one reading written", pass)
	}
	current := currentHealth(t, e, erp)
	if current == nil || current.State != crmcontracts.ProjectHealthStateAtRisk || current.Source == nil || *current.Source != "ai" {
		t.Fatalf("current reading = %+v, want at_risk from the ai", current)
	}
	if current.Note == nil || !strings.Contains(*current.Note, "30 days") {
		t.Fatalf("the reading's reason = %v, want the quiet rule's sentence", current.Note)
	}
	if by := e.WsScalar(t, `SELECT captured_by FROM project_health_assessment WHERE id = $1`, current.Id); by != "agent:project-health" {
		t.Fatalf("captured_by = %q, want the job's agent principal", by)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'project_health_assessment' AND entity_id = $1`, current.Id); n != 1 {
		t.Fatalf("audit rows for the reading = %d, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'project.health_assessed'
		AND envelope->'payload'->>'assessment_id' = $1::text`, current.Id); n != 1 {
		t.Fatalf("outbox rows for the reading = %d, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind = 'project_health_ai'
		AND target_type = 'project' AND target_id = $2 AND body LIKE '%30 days%'`, e.Rep1, erp); n != 1 {
		t.Fatalf("notices to the owner carrying the reason = %d, want 1", n)
	}

	// The job runs daily: the same silence writes nothing and tells nobody twice.
	if again := healthPass(t, e, now.Add(time.Hour)); again.Written != 0 {
		t.Fatalf("a repeat pass wrote %d readings, want none", again.Written)
	}
	if n := healthNotices(t, e, erp); n != 1 {
		t.Fatalf("notices after a repeat pass = %d, want 1", n)
	}
}

func TestAProjectWithNoOwnerGetsTheReadingAndNoNotice(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", nil, now)
	// No writer creates a project without an owner: a manual create stamps the
	// creator. An owner who left and was cleared is the case this stands for.
	e.WsExec(t, `UPDATE project SET owner_id = NULL WHERE id = $1`, erp)

	if pass := healthPass(t, e, now); pass.Written != 1 {
		t.Fatalf("pass = %+v, want the reading written", pass)
	}
	if n := healthNotices(t, e, erp); n != 0 {
		t.Fatalf("an ownerless project sent %d notices, want none", n)
	}
}

func TestAUsersReadingNewerThanTheLastActivityIsKept(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	said := now.AddDate(0, 0, -1)
	mine, err := e.Projects.RecordHealth(e.Admin(), projects.RecordHealthInput{
		ProjectID: erp, State: crmcontracts.ProjectHealthStateOnTrack, AssessedAt: &said,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pass := healthPass(t, e, now); pass.Written != 0 || pass.HeldByUser != 1 {
		t.Fatalf("pass = %+v, want the user's reading kept", pass)
	}
	if current := currentHealth(t, e, erp); current == nil || current.Id != mine.Id {
		t.Fatalf("current reading = %+v, want the user's %s", current, mine.Id)
	}

	// A call filed after the user spoke is the new activity. An open task
	// past its due date is what the job then finds. Filing a task does not
	// move the project's clock, so the call is needed beside it.
	fileTask(e.Admin(), t, e, erp, now.AddDate(0, 0, -2))
	fileActivity(e.Admin(), t, e, "call", now, &erp)
	if pass := healthPass(t, e, now.Add(time.Minute)); pass.Written != 1 {
		t.Fatalf("after new activity the pass = %+v, want the reading written", pass)
	}
	if current := currentHealth(t, e, erp); current == nil || current.Source == nil || *current.Source != "ai" ||
		current.State != crmcontracts.ProjectHealthStateAtRisk || !strings.Contains(*current.Note, "due date") {
		t.Fatalf("current reading = %+v, want the ai's at_risk naming the overdue task", current)
	}
}

func TestUndoingTheAIsReadingRestoresTheOneBeforeIt(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	before, note := now.AddDate(0, 0, -60), "Kick-off went well."
	mine, err := e.Projects.RecordHealth(e.Admin(), projects.RecordHealthInput{
		ProjectID: erp, State: crmcontracts.ProjectHealthStateOnTrack, Note: &note, AssessedAt: &before,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pass := healthPass(t, e, now); pass.Written != 1 {
		t.Fatalf("pass = %+v, want the ai's reading over the user's older one", pass)
	}
	ai := currentHealth(t, e, erp)

	undo, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(ai.Id))
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if undo.Source == nil || *undo.Source != "undo" || undo.SupersedesAssessmentId == nil || *undo.SupersedesAssessmentId != ai.Id {
		t.Fatalf("undo row = %+v, want source undo superseding the ai's reading", undo)
	}
	if current := currentHealth(t, e, erp); current == nil || current.Id != mine.Id || current.Note == nil || *current.Note != note {
		t.Fatalf("current reading after undo = %+v, want the user's %s with its note", current, mine.Id)
	}
	history := healthHistory(t, e, erp)
	if len(history) != 3 {
		t.Fatalf("history holds %d rows, want the user's, the ai's and the undo", len(history))
	}
	for _, row := range history {
		if row.Id == ai.Id && (row.Superseded == nil || !*row.Superseded) {
			t.Fatal("the ai's reading is not marked superseded after the undo")
		}
	}

	// The undo is a user speaking: the job stays off until new activity.
	if pass := healthPass(t, e, now.Add(time.Hour)); pass.Written != 0 || pass.HeldByUser != 1 {
		t.Fatalf("a pass after the undo = %+v, want it held off", pass)
	}
}

// Undoing the AI's first-ever reading leaves the project not judged. It does
// not read on track.
func TestUndoingTheOnlyReadingLeavesTheProjectNotJudged(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	if pass := healthPass(t, e, now); pass.Written != 1 {
		t.Fatalf("pass = %+v, want one reading", pass)
	}
	ai := currentHealth(t, e, erp)
	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(ai.Id)); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if current := currentHealth(t, e, erp); current != nil {
		t.Fatalf("current reading after undoing the only one = %+v, want none", current)
	}
}

func TestUndoRefusesWhatIsNotAStandingAIReading(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	other := quietDeliveringProject(t, e, "CRM rollout", &e.Rep1, now)
	if pass := healthPass(t, e, now); pass.Written != 2 {
		t.Fatalf("pass = %+v, want a reading on each project", pass)
	}
	ai, otherAI := currentHealth(t, e, erp), currentHealth(t, e, other)

	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(otherAI.Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("undoing another project's reading = %v, want not found", err)
	}
	undo, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(ai.Id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(ai.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undoing an undone reading = %v, want a conflict", err)
	}
	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(undo.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undoing an undo = %v, want a conflict", err)
	}
	mine, err := e.Projects.RecordHealth(e.Admin(), projects.RecordHealthInput{
		ProjectID: erp, State: crmcontracts.ProjectHealthStateOnTrack,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(mine.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undoing a user's reading = %v, want a conflict", err)
	}
	if _, err := e.Projects.CorrectHealth(e.Admin(), projects.CorrectHealthInput{
		ProjectID: erp, AssessmentID: ids.UUID(undo.Id), State: crmcontracts.ProjectHealthStateOnTrack,
	}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("correcting an undo row = %v, want a conflict", err)
	}
}

// A user's reading that commits while the job waits on the project lock is
// seen by the job, which then keeps it.
func TestAUsersReadingRacingTheJobIsKept(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	admin := e.Admin()

	held, release := make(chan struct{}), make(chan struct{})
	writer, written := make(chan error, 1), make(chan struct{})
	var holder int32
	go func() {
		defer close(written)
		writer <- database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
			if _, err := e.Projects.RecordHealthTx(admin, tx, projects.RecordHealthInput{
				ProjectID: erp, State: crmcontracts.ProjectHealthStateOnTrack,
			}); err != nil {
				return err
			}
			if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-writer:
		t.Fatalf("the user's write ended before it held the project: %v", err)
	}
	passed := make(chan struct{})
	var pass compose.ProjectHealthPass
	var passErr error
	go func() {
		defer close(passed)
		pass, passErr = compose.NewProjectHealthScan(e.Pool, func() time.Time { return now }).RunWorkspace(t.Context(), e.WS)
	}()
	var lift sync.Once
	t.Cleanup(func() {
		lift.Do(func() { close(release) })
		<-written
		<-passed
	})
	testdb.AwaitQueuedBehind(t, OwnerConn(t), holder, 1, passed, "the health job")
	lift.Do(func() { close(release) })
	if err := <-writer; err != nil {
		t.Fatal(err)
	}
	<-passed
	if passErr != nil {
		t.Fatal(passErr)
	}
	if pass.Written != 0 || pass.HeldByUser != 1 {
		t.Fatalf("the job racing a user's write = %+v, want it held off", pass)
	}
	if current := currentHealth(t, e, erp); current == nil || current.Source == nil || *current.Source != "human" {
		t.Fatalf("current reading = %+v, want the user's", current)
	}
}

// A user's reading and a correction leave through the outbox like the AI's.
func TestAUsersReadingAndCorrectionEmitTheirEvent(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	mine, err := e.Projects.RecordHealth(e.Admin(), projects.RecordHealthInput{
		ProjectID: erp, State: crmcontracts.ProjectHealthStateOnTrack,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixed := "Waiting on the customer's data."
	if _, err := e.Projects.CorrectHealth(e.Admin(), projects.CorrectHealthInput{
		ProjectID: erp, AssessmentID: ids.UUID(mine.Id), State: crmcontracts.ProjectHealthStateAtRisk, Note: &fixed,
	}); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'project.health_assessed'
		AND envelope->'payload'->>'source' = 'human'`); n != 2 {
		t.Fatalf("outbox rows for a reading and its correction = %d, want 2", n)
	}
}

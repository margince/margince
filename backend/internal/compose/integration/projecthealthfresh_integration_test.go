// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The health job judges from what is true under the project lock, not from
// what its listing saw. A project closed or touched since changes the outcome.
// So do an owner changed or gone, and an AI reading no longer in front of the
// owner.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// aiHealthWrite calls the job's write path for one project, as the job's
// agent. It reports the evidence the store read and whether the judge ran.
func aiHealthWrite(t *testing.T, e *Env, id ids.ProjectID, now time.Time) (projects.AIHealthResult, bool) {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(t.Context(), e.WS), "agent:project-health")
	store := projects.NewStore(e.DB()).SetClock(func() time.Time { return now })
	judged := false
	var result projects.AIHealthResult
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		result, err = store.RecordAIHealthTx(ctx, tx, id,
			func(context.Context, pgx.Tx, projects.AIHealthEvidence) (projects.AIHealthJudgement, error) {
				judged = true
				return projects.AIHealthJudgement{State: "at_risk", Note: "Quiet."}, nil
			})
		return err
	}); err != nil {
		t.Fatalf("the job's write: %v", err)
	}
	return result, judged
}

func TestAProjectClosedSinceTheListingGetsNoReading(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	reason := "Handed over."
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), erp, projects.AdvanceProjectPhaseInput{
		ToPhase: projects.PhaseClosed, Reason: &reason,
	}); err != nil {
		t.Fatal(err)
	}
	result, judged := aiHealthWrite(t, e, erp, now)
	if result.Outcome != projects.AIHealthNotInFlight || judged {
		t.Fatalf("a closed project = %+v (judged %t), want no reading and no judgement", result, judged)
	}
}

func TestAProjectTouchedSinceTheListingIsJudgedAsActive(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	fileActivity(e.Admin(), t, e, "call", now, &erp)
	if pass := healthPass(t, e, now); pass.Written != 0 {
		t.Fatalf("a project with a call today = %+v, want no reading", pass)
	}
	result, _ := aiHealthWrite(t, e, erp, now)
	if result.Evidence.Quiet {
		t.Fatal("the evidence read under the lock calls a project with a call today quiet")
	}
}

// An owner handed the project while the job waits on its lock is the one told.
func TestAnOwnerChangedWhileTheJobWaitsIsTheOneTold(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	admin := e.Admin()

	held, release := make(chan struct{}), make(chan struct{})
	writer, written := make(chan error, 1), make(chan struct{})
	var holder int32
	go func() {
		defer close(written)
		writer <- database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
			if _, err := projects.TransferProjectOwnershipTx(admin, tx, projects.TransferProjectOwnershipInput{
				FromOwnerID: ids.From[ids.UserKind](e.Rep1), ToOwnerID: ids.From[ids.UserKind](e.Rep2),
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
		t.Fatalf("the transfer ended before it held the project: %v", err)
	}
	passed := make(chan struct{})
	var passErr error
	go func() {
		defer close(passed)
		_, passErr = compose.NewProjectHealthScan(e.Pool, func() time.Time { return now }).RunWorkspace(t.Context(), e.WS)
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
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'project_health_ai' AND recipient_user_id = $1`, e.Rep2); n != 1 {
		t.Fatalf("notices to the new owner = %d, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind = 'project_health_ai' AND recipient_user_id = $1`, e.Rep1); n != 0 {
		t.Fatalf("notices to the former owner = %d, want none", n)
	}
}

func TestADeactivatedOwnerIsNotTold(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	deactivate(t, e, e.Rep1)
	if pass := healthPass(t, e, now); pass.Written != 1 {
		t.Fatalf("pass = %+v, want the reading written", pass)
	}
	if n := healthNotices(t, e, erp); n != 0 {
		t.Fatalf("a deactivated owner was sent %d notices, want none", n)
	}
}

// Only the reading in front of the owner can be undone: an older AI reading
// under a newer one is refused.
func TestUndoRefusesAnAIReadingThatNoLongerStands(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP replacement", &e.Rep1, now)
	if pass := healthPass(t, e, now); pass.Written != 1 {
		t.Fatalf("pass = %+v, want the first reading", pass)
	}
	older := currentHealth(t, e, erp)
	fileTask(e.Admin(), t, e, erp, now.AddDate(0, 0, -2))
	if pass := healthPass(t, e, now.Add(time.Hour)); pass.Written != 1 {
		t.Fatalf("pass = %+v, want the off_track reading over it", pass)
	}
	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(older.Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undoing an AI reading under a newer one = %v, want a conflict", err)
	}
	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(currentHealth(t, e, erp).Id)); err != nil {
		t.Fatalf("undoing the standing AI reading: %v", err)
	}
}

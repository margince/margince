// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func phaseLinkProject(t *testing.T, e *integration.Env, name string) ids.ProjectID {
	t.Helper()
	company := e.SeedCompany(t, name+" GmbH", nil)
	made, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: name, CompanyID: ids.From[ids.CompanyKind](company), Source: "manual",
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	id := ids.From[ids.ProjectKind](ids.UUID(made.Id))
	reason := "kick-off signed"
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), id, projects.AdvanceProjectPhaseInput{
		ToPhase: projects.PhasePursuing, Reason: &reason,
	}); err != nil {
		t.Fatalf("advance %s: %v", name, err)
	}
	return id
}

// phaseRowsOf renders the columns the task must leave alone, with the link.
func phaseRowsOf(t *testing.T, e *integration.Env, project ids.ProjectID, columns string) string {
	t.Helper()
	return e.WsScalar(t, `SELECT COALESCE(string_agg(`+columns+`, ' | ' ORDER BY occurred_at, id), '')
		FROM project_phase_history WHERE project_id = $1`, project)
}

const phaseWriterColumns = `concat_ws(',', changed_by, COALESCE(reason, '-'), occurred_at::text)`

func TestThePhaseHistoryLinkTaskLinksOnlyWhatOneAuditRowProves(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	linked := phaseLinkProject(t, e, "Linked")
	ambiguous := phaseLinkProject(t, e, "Ambiguous")
	orphan := phaseLinkProject(t, e, "Orphan")
	e.WsExec(t, `UPDATE project_phase_history SET audit_log_id = NULL`)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(t.Context(), `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		SELECT actor_type, actor_id, action, entity_type, entity_id, occurred_at FROM audit_log
		 WHERE entity_type = 'project' AND entity_id = $1 AND action = 'advance_phase'`, ambiguous); err != nil {
		t.Fatalf("duplicating the audit row: %v", err)
	}
	// The move's instant no longer equals any audit row's, as for a row whose
	// audit row is gone.
	e.WsExec(t, `UPDATE project_phase_history SET occurred_at = occurred_at + interval '1 second'
		WHERE project_id = $1 AND from_phase IS NOT NULL`, orphan)

	before := phaseRowsOf(t, e, linked, phaseWriterColumns) + phaseRowsOf(t, e, ambiguous, phaseWriterColumns) +
		phaseRowsOf(t, e, orphan, phaseWriterColumns)
	task := phaseHistoryLinksDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runMachineAddressTask(ctx, t, e, task)

	if got := phaseRowsOf(t, e, linked, `(audit_log_id IS NOT NULL)::text`); got != "true | true" {
		t.Errorf("linked project flags = %q, want both rows linked", got)
	}
	if got := phaseRowsOf(t, e, ambiguous, `(audit_log_id IS NOT NULL)::text`); got != "true | false" {
		t.Errorf("ambiguous project flags = %q, want the birth row linked and the move unlinked", got)
	}
	if got := phaseRowsOf(t, e, orphan, `(audit_log_id IS NOT NULL)::text`); got != "true | false" {
		t.Errorf("orphan project flags = %q, want the birth row linked and the move unlinked", got)
	}
	if mismatched := e.WsCount(t, `SELECT count(*) FROM project_phase_history h JOIN audit_log a ON a.id = h.audit_log_id
		WHERE a.entity_id <> h.project_id OR a.occurred_at <> h.occurred_at`); mismatched != 0 {
		t.Errorf("%d links name an audit row of another project or instant", mismatched)
	}
	after := phaseRowsOf(t, e, linked, phaseWriterColumns) + phaseRowsOf(t, e, ambiguous, phaseWriterColumns) +
		phaseRowsOf(t, e, orphan, phaseWriterColumns)
	if after != before {
		t.Errorf("the task changed what the writer recorded:\n before %s\n after  %s", before, after)
	}

	var replay datatask.BatchOut
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		replay, err = task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if replay.Changed != 0 {
		t.Errorf("a second run changed %d rows, want none", replay.Changed)
	}
}

func TestThePhaseHistoryLinkTaskEstimateIsBoundedAndPagesPastUnmatchedRows(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	for _, name := range []string{"One", "Two", "Three"} {
		phaseLinkProject(t, e, name)
	}
	e.WsExec(t, `UPDATE project_phase_history SET audit_log_id = NULL`)
	step := phaseHistoryLinksDataTask().Steps[0]
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		count, more, err := step.Estimate(ctx, tx, 2)
		if err != nil || count != 2 || !more {
			t.Errorf("estimate with a cap of 2 = %d, more=%v, err=%v; want 2 and more", count, more, err)
		}
		in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
		scanned := 0
		for range 10 {
			out, err := step.Batch(ctx, tx, in)
			if err != nil {
				return err
			}
			scanned += out.Scanned
			if out.Done {
				break
			}
			in.Cursor = out.Cursor
		}
		if scanned != 6 {
			t.Errorf("paging inspected %d rows, want all 6 (a birth row and a move for each of 3 projects)", scanned)
		}
		if _, err := step.Batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`{`), Limit: 2}); err == nil {
			t.Error("a malformed cursor was accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

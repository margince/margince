// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The signal day-count task rewords the summaries the scan stored with the day
// count of its first pass. The fixtures are written through the signals
// writer with the old wording, since the scan no longer produces it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

type legacySignal struct {
	kind, summary, source string
	project               ids.UUID
}

// seedLegacySignal writes one signal on the company and returns its id.
func seedLegacySignal(ctx context.Context, t *testing.T, e *integration.Env, company ids.UUID, s legacySignal) ids.UUID {
	t.Helper()
	fingerprint := "day-counts:" + ids.NewV7().String()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind: s.kind, CompanyID: company, ProjectID: s.project, Summary: s.summary,
			Severity: severityWarn, Fingerprint: fingerprint, Source: s.source,
		}, time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids.MustParse(e.WsScalar(t, `SELECT id::text FROM signal WHERE fingerprint = $1`, fingerprint))
}

func runSignalDayCountTask(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) {
	t.Helper()
	runCalendarDescriptionTask(ctx, t, e, task)
}

func TestTheSignalDayCountTaskRewordsOnlyTheScansOldSummaries(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	company := e.SeedCompany(t, "Day Count Co", nil)
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "ERP rollout", CompanyID: ids.From[ids.CompanyKind](company), OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	project := ids.UUID(created.Id)
	legacy := func(lang textlang.Lang) signalSummaryCopy { return legacySignalSummaryByLang[lang] }
	said := signalSummaryCopyFor

	english := seedLegacySignal(ctx, t, e, company, legacySignal{kind: kindGhostedThread, summary: fmt.Sprintf(legacy(textlang.English).ghostedThread, 15)})
	german := seedLegacySignal(ctx, t, e, company, legacySignal{kind: kindGhostedThread, summary: fmt.Sprintf(legacy(textlang.German).ghostedThread, 22)})
	quiet := seedLegacySignal(ctx, t, e, company, legacySignal{
		kind: kindProjectGoneQuiet, project: project, summary: fmt.Sprintf(legacy(textlang.Vietnamese).projectQuiet, "ERP rollout", 45),
	})
	current := fmt.Sprintf(said(textlang.English).ghostedThread, ghostedThresholdDays)
	reworded := seedLegacySignal(ctx, t, e, company, legacySignal{kind: kindGhostedThread, summary: current})
	edited := "We wrote 15 days ago and nobody has answered. Chase the CFO."
	other := seedLegacySignal(ctx, t, e, company, legacySignal{kind: kindGhostedThread, summary: edited})
	human := fmt.Sprintf(legacy(textlang.English).ghostedThread, 9)
	filed := seedLegacySignal(ctx, t, e, company, legacySignal{kind: kindGhostedThread, summary: human, source: "analyst-note"})

	fingerprints := map[ids.UUID]string{}
	for _, id := range []ids.UUID{english, german, quiet} {
		fingerprints[id] = e.WsScalar(t, `SELECT fingerprint FROM signal WHERE id = $1`, id)
	}
	if got := signalDayCountEstimate(ctx, t, e); got != 5 {
		t.Errorf("the estimate counts %d scan signals, want 5", got)
	}

	task := signalDayCountsDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runSignalDayCountTask(ctx, t, e, task)

	for name, c := range map[string]struct {
		id   ids.UUID
		want string
	}{
		"English ghosted":  {english, current},
		"German ghosted":   {german, fmt.Sprintf(said(textlang.German).ghostedThread, ghostedThresholdDays)},
		"Vietnamese quiet": {quiet, fmt.Sprintf(said(textlang.Vietnamese).projectQuiet, "ERP rollout", projects.DefaultProjectQuietDays)},
	} {
		if got := e.WsScalar(t, `SELECT summary FROM signal WHERE id = $1`, c.id); got != c.want {
			t.Errorf("%s summary = %q, want %q", name, got, c.want)
		}
		if got := e.WsScalar(t, `SELECT fingerprint FROM signal WHERE id = $1`, c.id); got != fingerprints[c.id] {
			t.Errorf("%s fingerprint changed to %q", name, got)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1
			AND action = 'update' AND actor_type = 'system' AND after ? 'summary'`, c.id); got != 1 {
			t.Errorf("%s reword audit rows = %d, want 1", name, got)
		}
	}
	for name, c := range map[string]struct {
		id   ids.UUID
		want string
	}{"already reworded": {reworded, current}, "edited": {other, edited}, "not the scan's": {filed, human}} {
		if got := e.WsScalar(t, `SELECT summary FROM signal WHERE id = $1`, c.id); got != c.want {
			t.Errorf("%s summary changed to %q", name, got)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1 AND action = 'update'`, c.id); got != 0 {
			t.Errorf("%s carries %d update audit rows", name, got)
		}
	}

	// A replay from the start finds nothing in the old wording.
	step := signalDayCountsDataTask().Steps[0]
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = step.Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay reworded %d more signals", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
	in = datatask.BatchIn{Limit: 2, RunID: ids.NewV7(), Cursor: json.RawMessage(`"not-a-uuid"`)}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := step.Batch(ctx, tx, in)
		return err
	}); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}

func signalDayCountEstimate(ctx context.Context, t *testing.T, e *integration.Env) int64 {
	t.Helper()
	count, _, err := signalDayCountsDataTask().Steps[0].Estimate(ctx, e.Pool, 100)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestTheSignalDayCountTaskRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	company := e.SeedCompany(t, "Rollback Count Co", nil)
	stored := fmt.Sprintf(legacySignalSummaryByLang[textlang.English].ghostedThread, 15)
	signal := seedLegacySignal(ctx, t, e, company, legacySignal{kind: kindGhostedThread, summary: stored})

	task := signalDayCountsDataTask()
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runSignalDayCountTask(ctx, t, e, task)

	if got := e.WsScalar(t, `SELECT summary FROM signal WHERE id = $1`, signal); got != stored {
		t.Errorf("the reword survived a rolled-back batch: %q", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1 AND action = 'update'`, signal); got != 0 {
		t.Errorf("reword audit rows escaped the rollback: %d", got)
	}
}

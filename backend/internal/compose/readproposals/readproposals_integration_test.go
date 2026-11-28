// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package readproposals

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func systemCtx(e *integration.Env) context.Context {
	return principal.SystemActing(e.Admin(), "system:datatask-test")
}

// approval stages one approval through the approvals module, as a read does.
func approval(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	id, err := approvals.NewService(e.DB()).Stage(e.AgentCtx(), approvals.StageInput{
		Kind: "disqualify_lead", ProposedChange: []byte(`{"lead_id":"x"}`),
		DiffHash: "read-proposal-hash", Summary: "Disqualify this lead?",
	})
	if err != nil {
		t.Fatalf("staging an approval: %v", err)
	}
	return id.UUID
}

// olderSiteRead plants a finished site read whose list sits only in
// proposal_ids, as every read finished before the keyed table existed.
func olderSiteRead(t *testing.T, e *integration.Env, list ...ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO site_read (id, target_kind, seed_url, requested_by, status, proposal_ids)
		VALUES ($1, 'onboarding', 'https://acme.example', 'human:x', 'done', $2)`, id, append([]ids.UUID{}, list...))
	return id
}

// olderTranscriptRead plants a finished transcript read the same way.
func olderTranscriptRead(t *testing.T, e *integration.Env, list ...ids.UUID) ids.UUID {
	t.Helper()
	activity, id := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Fleet renewal', now(), 'manual', 'human:x')`, activity)
	e.WsExec(t, `INSERT INTO transcript_read (id, activity_id, requested_by, status, started_at, finished_at, proposal_ids)
		VALUES ($1, $2, 'human:x', 'done', now(), now(), $3)`, id, activity, append([]ids.UUID{}, list...))
	return id
}

// keyed reads one read's keyed rows in position order.
func keyed(t *testing.T, e *integration.Env, table string, read ids.UUID) []ids.UUID {
	t.Helper()
	column := map[string]string{"site_read_proposal": "site_read_id", "transcript_read_proposal": "transcript_read_id"}[table]
	var list []ids.UUID
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT ARRAY(SELECT approval_id FROM `+pgx.Identifier{table}.Sanitize()+
			` WHERE `+pgx.Identifier{column}.Sanitize()+` = $1 ORDER BY position)`, read).Scan(&list)
	}); err != nil {
		t.Fatalf("reading the keyed rows of %s: %v", table, err)
	}
	return list
}

// Each older list gains its keyed rows in the order it staged them. A repeat
// keeps its first place, and an id whose approval is gone is left out. A row
// already keyed stays as it was, and a second pass changes nothing.
func TestTheTaskKeysEveryOlderListAndReplaysAsANoOp(t *testing.T) {
	e := integration.Setup(t)
	first, second, third := approval(t, e), approval(t, e), approval(t, e)
	gone := ids.NewV7()
	repeated := olderSiteRead(t, e, first, gone, second, first)
	empty := olderSiteRead(t, e)
	alreadyKeyed := olderSiteRead(t, e, third)
	e.WsExec(t, `INSERT INTO site_read_proposal (site_read_id, approval_id, position) VALUES ($1, $2, 1)`, alreadyKeyed, third)
	transcript := olderTranscriptRead(t, e, third, first, third)

	ctx := systemCtx(e)
	task := DataTask()
	if n, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 2); err != nil || n != 2 || !capped {
		t.Fatalf("the site read estimate under a cap of 2 = %d, capped %v (%v), want 2 and capped", n, capped, err)
	}
	if n, capped, err := task.Steps[1].Estimate(ctx, e.Pool, 2); err != nil || n != 1 || capped {
		t.Fatalf("the transcript read estimate under a cap of 2 = %d, capped %v (%v), want 1 and not capped", n, capped, err)
	}
	e.RunDataTask(ctx, t, task)

	for _, c := range []struct {
		name, table string
		read        ids.UUID
		want        []ids.UUID
	}{
		{"a repeat and a gone approval", "site_read_proposal", repeated, []ids.UUID{first, second}},
		{"an empty list", "site_read_proposal", empty, []ids.UUID{}},
		{"a list already keyed", "site_read_proposal", alreadyKeyed, []ids.UUID{third}},
		{"a transcript read", "transcript_read_proposal", transcript, []ids.UUID{third, first}},
	} {
		if got := keyed(t, e, c.table, c.read); !slices.Equal(got, c.want) {
			t.Errorf("%s: keyed %v, want %v", c.name, got, c.want)
		}
	}

	for _, step := range task.Steps {
		if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 100})
			if err == nil && (out.Changed != 0 || !out.Done) {
				t.Errorf("%s: a replay changed %d read(s), done %v, want none and done", step.Name, out.Changed, out.Done)
			}
			return err
		}); err != nil {
			t.Fatalf("replaying %s: %v", step.Name, err)
		}
	}
}

// Through the real executor, batches of two walk every read in key order,
// including the reads that hold no list.
func TestBatchesOfTwoReachEveryRead(t *testing.T) {
	e := integration.Setup(t)
	staged := approval(t, e)
	var reads []ids.UUID
	for i := range 5 {
		if i == 2 {
			olderSiteRead(t, e)
			continue
		}
		reads = append(reads, olderSiteRead(t, e, staged))
	}
	task := DataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 2, 1
	e.RunDataTask(systemCtx(e), t, task)
	for _, read := range reads {
		if got := keyed(t, e, "site_read_proposal", read); !slices.Equal(got, []ids.UUID{staged}) {
			t.Errorf("read %s keyed %v, want [%s]", read, got, staged)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM data_task_run
		WHERE task_id = $1 AND step = 'copy_site_read_proposals' AND status = 'done' AND scanned = 5 AND changed = 4 AND batches = 3`,
		task.ID); n != 1 {
		t.Error("the site read step did not finish in 3 batches having inspected 5 reads and changed 4")
	}
}

// A batch that fails after its copy leaves no keyed row, and a cursor that is
// no key is refused.
func TestTheTaskRollsBackWithItsBatchAndRefusesAMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	read := olderTranscriptRead(t, e, approval(t, e))
	ctx := systemCtx(e)
	task := DataTask()
	task.Steps = task.Steps[1:]
	copyBatch := task.Steps[0].Batch
	interrupted := false
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := copyBatch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		interrupted = out.Changed == 1
		return out, errors.New("interrupt after the copy")
	}
	e.RunDataTask(ctx, t, task)
	if !interrupted {
		t.Fatal("the batch failed before its copy, so no rollback was tested")
	}
	if got := keyed(t, e, "transcript_read_proposal", read); len(got) != 0 {
		t.Errorf("the rolled-back batch left keyed rows %v", got)
	}
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := copyBatch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 2})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
}

// Only the system actor the executor supplies may copy, and only a reader of
// data tasks may estimate.
func TestACopyRefusesAHumanAndAnEstimateRefusesAStranger(t *testing.T) {
	e := integration.Setup(t)
	human := e.As(e.Rep1, nil, integration.AdminPerms)
	stranger := e.As(e.Rep1, nil, principal.Permissions{})
	for _, step := range DataTask().Steps {
		if err := database.WithTx(human, e.Pool, func(tx pgx.Tx) error {
			_, err := step.Batch(human, tx, datatask.BatchIn{Limit: 2})
			return err
		}); err == nil {
			t.Errorf("%s ran for a human", step.Name)
		}
		if _, _, err := step.Estimate(stranger, e.Pool, 10); err == nil {
			t.Errorf("%s estimated for a member with no data task grant", step.Name)
		}
	}
}

// A copy or a count whose statement cannot run reports the failure instead of
// an empty page that would finish the step.
func TestACancelledCopyOrCountIsAnError(t *testing.T) {
	e := integration.Setup(t)
	olderSiteRead(t, e, approval(t, e))
	cancelled, cancel := context.WithCancel(systemCtx(e))
	cancel()
	for _, step := range DataTask().Steps {
		if _, _, err := step.Estimate(cancelled, e.Pool, 10); err == nil {
			t.Errorf("%s estimated on a cancelled context", step.Name)
		}
		if err := database.WithTx(systemCtx(e), e.Pool, func(tx pgx.Tx) error {
			_, err := step.Batch(cancelled, tx, datatask.BatchIn{Limit: 2})
			return err
		}); err == nil {
			t.Errorf("%s copied on a cancelled context", step.Name)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM site_read_proposal`); got != 0 {
		t.Errorf("a cancelled copy left %d keyed row(s)", got)
	}
}

// The key refuses a writer that names an approval that does not exist. The
// read stays running, with neither its list nor a keyed row written.
func TestAFinishNamingAMissingApprovalIsRefused(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	missing := []ids.UUID{ids.NewV7()}

	site := ids.NewV7()
	e.WsExec(t, `INSERT INTO site_read (id, target_kind, seed_url, requested_by, status, started_at)
		VALUES ($1, 'onboarding', 'https://acme.example', 'human:x', 'running', now())`, site)
	if err := contacts.NewStore(e.DB()).FinishSiteRead(ctx, site, contacts.FinishSiteReadInput{
		Status: "done", ProposalIDs: missing,
	}); err == nil {
		t.Error("a site read finished naming an approval that does not exist")
	}

	transcript := olderTranscriptRead(t, e)
	e.WsExec(t, `UPDATE transcript_read SET status = 'running', finished_at = NULL WHERE id = $1`, transcript)
	if err := activities.NewStore(e.DB()).FinishTranscriptRead(ctx, transcript, activities.TranscriptReadOutcome{
		Status: activities.TranscriptReadDone, ProposalIDs: missing,
	}); err == nil {
		t.Error("a transcript read finished naming an approval that does not exist")
	}

	if n := e.WsCount(t, `SELECT (SELECT count(*) FROM site_read WHERE id = $1 AND status = 'running' AND proposal_ids = '{}')
		+ (SELECT count(*) FROM transcript_read WHERE id = $2 AND status = 'running' AND proposal_ids = '{}')
		- (SELECT count(*) FROM site_read_proposal) - (SELECT count(*) FROM transcript_read_proposal)`, site, transcript); n != 2 {
		t.Error("a refused finish left a read finished, a list written or a keyed row behind")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func systemCtx(e *integration.Env) context.Context {
	return principal.SystemActing(e.Admin(), "system:datatask-test")
}

// older plants deals, offers and projects the way rows were written before the
// source checks existed.
type older struct {
	t        *testing.T
	e        *integration.Env
	owner    *pgx.Conn
	pipeline ids.PipelineID
	stage    ids.StageID
}

func seedOlder(t *testing.T) older {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	return older{t: t, e: e, owner: integration.OwnerConn(t), pipeline: pipeline, stage: open}
}

func (o older) deal(source string) ids.UUID {
	o.t.Helper()
	return integration.SeedBeforeSourceCheck(o.t, o.owner, "deal",
		`INSERT INTO deal (id, name, status, pipeline_id, stage_id, source, captured_by)
		 VALUES ($1, 'Older deal', 'open', $2, $3, $4, 'human:x')`, o.pipeline, o.stage, source)
}

func (o older) offer(deal ids.UUID, number, source string) ids.UUID {
	o.t.Helper()
	return integration.SeedBeforeSourceCheck(o.t, o.owner, "offer",
		`INSERT INTO offer (id, deal_id, offer_number, revision, status, currency, source, captured_by)
		 VALUES ($1, $2, $3, 1, 'draft', 'EUR', $4, 'human:x')`, deal, number, source)
}

func (o older) project(key, source string) ids.UUID {
	o.t.Helper()
	return integration.SeedBeforeSourceCheck(o.t, o.owner, "project",
		`INSERT INTO project (id, name, key, phase, source, captured_by)
		 VALUES ($1, 'Older project', $2, 'delivering', $3, 'human:x')`, key, source)
}

func (o older) source(table string, id ids.UUID) string {
	o.t.Helper()
	var source string
	if err := o.owner.QueryRow(context.Background(),
		`SELECT source FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1`, id).Scan(&source); err != nil {
		o.t.Fatalf("reading the source of %s %s: %v", table, id, err)
	}
	return source
}

// Each shape a writer stored before the list lands on its channel, and the
// audit row keeps the value it replaced.
func TestTheTaskMovesEveryOlderSourceOntoItsChannel(t *testing.T) {
	o := seedOlder(t)
	channels := map[string]string{
		"mirror:csv:deal:42": "import", "ext:finance:erp": "import", "agent:dealscout": "automation",
		"lead:018f0000-0000-7000-8000-000000000001": "manual", "ui": "manual", "seed": "api", "": "api",
	}
	deals := map[string]ids.UUID{}
	for source := range channels {
		deals[source] = o.deal(source)
	}
	typed := o.e.SeedDeal(t, "Typed deal", o.pipeline, o.stage, nil)
	offer := o.offer(typed, "A-1", "acme-helpdesk:ticket-4471")
	project := o.project("OLD1", "mcp")

	ctx := systemCtx(o.e)
	if n, capped, err := DataTask().Steps[0].Estimate(ctx, o.e.Pool, 3); err != nil || n != 3 || !capped {
		t.Fatalf("the deal estimate under a cap of 3 = %d, capped %v (%v), want 3 and capped", n, capped, err)
	}
	o.e.RunDataTask(ctx, t, DataTask())

	for source, want := range channels {
		if got := o.source("deal", deals[source]); got != want {
			t.Errorf("a deal stored with source %q now reads %q, want %q", source, got, want)
		}
	}
	if got := o.source("offer", offer); got != "api" {
		t.Errorf("the offer's source = %q, want api", got)
	}
	if got := o.source("project", project); got != "manual" {
		t.Errorf("the project's source = %q, want manual", got)
	}
	if got := o.source("deal", typed); got != "manual" {
		t.Errorf("a deal already on the list was moved to %q", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1
		AND action = 'update' AND before->>'source' = 'mirror:csv:deal:42' AND after->>'source' = 'import'
		AND evidence ? 'data_task_run'`, deals["mirror:csv:deal:42"]); n != 1 {
		t.Errorf("%d audit row(s) keep the imported deal's older source, want 1", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM event_outbox o
		JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		WHERE a.entity_id = ANY($1) AND o.envelope->>'type' IN ('deal.updated', 'project.updated')
		AND a.after->>'source' IS NOT NULL`, []ids.UUID{deals["seed"], project}); n != 2 {
		t.Errorf("%d update event(s) announce the corrected deal and project, want 2", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM event_outbox o
		WHERE o.envelope->'entity'->>'id' = $1`, offer.String()); n != 1 {
		t.Errorf("the corrected offer sent %d event(s), want its source update", n)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'`, typed); n != 0 {
		t.Errorf("a deal already on the list got %d audit row(s)", n)
	}
	for i, step := range DataTask().Steps {
		if n, _, err := step.Estimate(ctx, o.e.Pool, 10); err != nil || n != 0 {
			t.Errorf("step %d still estimates %d row(s) (%v), want none", i, n, err)
		}
	}
}

// Batches of two walk the table in key order until every older deal is moved,
// and a replay changes nothing.
func TestBatchesOfTwoReachEveryOlderDealAndAReplayChangesNothing(t *testing.T) {
	o := seedOlder(t)
	for range 5 {
		o.deal("seed")
	}
	ctx := systemCtx(o.e)
	move := DataTask().Steps[0].Batch
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for batches := 1; ; batches++ {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) (err error) {
			out, err = move(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatalf("batch %d: %v", batches, err)
		}
		if out.Changed > 2 {
			t.Fatalf("batch %d moved %d deals, over its limit of 2", batches, out.Changed)
		}
		if out.Done {
			break
		}
		if batches > 5 {
			t.Fatal("the walk did not finish in the batches five deals need")
		}
		in.Cursor = out.Cursor
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM deal WHERE source = 'seed'`); n != 0 {
		t.Errorf("%d older deal(s) were never reached", n)
	}
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		out, err := move(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || !out.Done) {
			t.Errorf("a replay changed %d deal(s), done %v, want none and done", out.Changed, out.Done)
		}
		return err
	}); err != nil {
		t.Fatalf("replaying the batch: %v", err)
	}
}

// A batch that fails after its move leaves the deal and the audit log as they
// were. A cursor that is no key is refused, and so is a caller that is no system.
func TestTheTaskRollsBackWithItsBatchAndRefusesAMalformedCursor(t *testing.T) {
	o := seedOlder(t)
	deal := o.deal("seed")
	ctx := systemCtx(o.e)
	task := DataTask()
	move := task.Steps[0].Batch
	interrupted := false
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := move(ctx, tx, in)
		if err != nil {
			return out, err
		}
		interrupted = out.Changed == 1
		return out, errors.New("interrupt after the move")
	}
	o.e.RunDataTask(ctx, t, task)
	if !interrupted {
		t.Fatal("the batch failed before its move, so no rollback was tested")
	}
	if got := o.source("deal", deal); got != "seed" {
		t.Errorf("the rolled-back batch left source %q, want seed", got)
	}
	if n := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'`, deal); n != 0 {
		t.Errorf("the rolled-back batch left %d audit row(s)", n)
	}
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		_, err := move(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 2})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
	member := o.e.Admin()
	if err := database.WithWorkspaceTx(member, o.e.Pool, func(tx pgx.Tx) error {
		_, err := move(member, tx, datatask.BatchIn{Limit: 2})
		return err
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a member running the correction: err = %v, want permission denied", err)
	}
}

// An unvalidated check still judges an older row when it is next written. An
// edit before the task has run goes through, and leaves the row on its channel.
func TestAnEditBeforeTheTaskMovesAnOlderRowOntoItsChannel(t *testing.T) {
	o := seedOlder(t)
	deal := o.deal("mirror:csv:deal:7")
	for table, id := range map[string]ids.UUID{
		"deal": deal, "offer": o.offer(deal, "A-7", "agent:dealscout"), "project": o.project("OLD7", "seed"),
	} {
		if _, err := o.owner.Exec(context.Background(),
			`UPDATE `+pgx.Identifier{table}.Sanitize()+` SET updated_at = now() WHERE id = $1`, id); err != nil {
			t.Fatalf("editing an older %s: %v", table, err)
		}
	}
	for table, want := range map[string]string{"deal": "import", "offer": "automation", "project": "api"} {
		if n := o.e.WsCount(t, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()+` WHERE source = $1`, want); n != 1 {
			t.Errorf("%d %s row(s) read %q after the edit, want 1", n, table, want)
		}
	}
	if err := o.e.WsExecErr(t, `UPDATE deal SET source = 'seed' WHERE id = $1`, deal); err != nil {
		t.Fatalf("an update naming an unlisted source: %v", err)
	}
	if got := o.source("deal", deal); got != "api" {
		t.Errorf("an update naming an unlisted source stored %q, want its channel api", got)
	}
}

// A new revision copies the offer it revises. An older offer's revision takes
// the channel, so revising it does not wait for the task.
func TestARevisionOfAnOlderOfferTakesItsChannel(t *testing.T) {
	o := seedOlder(t)
	deal := o.e.SeedDeal(t, "Quoted deal", o.pipeline, o.stage, nil)
	sent := integration.SeedBeforeSourceCheck(t, o.owner, "offer",
		`INSERT INTO offer (id, deal_id, offer_number, revision, status, currency, source, captured_by)
		 VALUES ($1, $2, 'A-9', 1, 'sent', 'EUR', 'mirror:csv:offer:9', 'human:x')`, deal)

	desk := o.e.As(o.e.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true, Update: true}, "offer": {Create: true, Read: true, Update: true},
			"company": {Read: true}, "installation_settings": {Read: true},
		},
	})
	revision, err := o.e.Deals.RegenerateOffer(desk, ids.From[ids.OfferKind](sent))
	if err != nil {
		t.Fatalf("revising an older offer: %v", err)
	}
	if revision.Source != "import" {
		t.Errorf("the revision's source = %q, want import", revision.Source)
	}
}

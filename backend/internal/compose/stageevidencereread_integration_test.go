// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Removing an own domain queues the stage evidence reading again for the mail
// that domain sent. The removal goes through the admin list's store, and the
// jobs it queues are read from River's table and driven directly.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const budgetConfirmedText = "We confirm the budget."

// budgetConfirmedLane is a model that reads a met budget confirmation in the
// activity it is pointed at, and counts how often it was asked.
type budgetConfirmedLane struct {
	about ids.UUID
	calls int
}

func (l *budgetConfirmedLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	// The prompt names the activity by a ref it mints, and a claim cites that.
	source := l.about.String()
	ref := stageEvidenceSpanRefs([]stageEvidenceSpan{{SourceID: source}}).For(source)
	return model.Response{Text: fmt.Sprintf(`{"claims":[{"criterion_key":"budget","source_id":%q,
		"source_lines":[1],"quote":%q,"met":"true","commitment":"agreed","confidence":0.9}]}`,
		ref, budgetConfirmedText)}, nil
}

type rereadFixture struct {
	*authorshipFixture
	lane *budgetConfirmedLane
}

func newRereadFixture(t *testing.T) *rereadFixture {
	t.Helper()
	f := newAuthorshipFixture(t)
	integration.ApplyRiverSchema(t)
	f.own = f.own.WithRemoved(queueStageEvidenceReread)
	return &rereadFixture{authorshipFixture: f, lane: &budgetConfirmedLane{}}
}

// mail is inbound mail from the sender that confirms the budget.
func (f *rereadFixture) mail(sender string) ids.UUID {
	f.t.Helper()
	id := f.inboundMail(sender)
	f.WsExec(f.t, `UPDATE activity SET body = $2 WHERE id = $1`, id, budgetConfirmedText)
	return id
}

// read runs one queued reading the way the worker role runs it.
func (f *rereadFixture) read(args StageEvidenceReadArgs) {
	f.t.Helper()
	f.lane.about = args.ActivityID
	worker := newStageEvidenceReadWorker(f.Pool, f.lane, slog.New(slog.DiscardHandler))
	if err := worker.Work(context.Background(), &river.Job[StageEvidenceReadArgs]{Args: args}); err != nil {
		f.t.Fatalf("reading activity %s: %v", args.ActivityID, err)
	}
}

// firstReading queues the reading the way the trigger does when mail lands,
// and runs it. Its row stays in River's table, as a finished job's does.
func (f *rereadFixture) firstReading(mail ids.UUID) {
	f.t.Helper()
	args := StageEvidenceReadArgs{Workspace: f.WS, DealID: f.dealID.UUID, ActivityID: mail}
	queue, err := callerTxJobs()
	if err != nil {
		f.t.Fatal(err)
	}
	if err := pgx.BeginFunc(context.Background(), f.Pool, func(tx pgx.Tx) error {
		return queue.EnqueueTx(context.Background(), tx, args, stageEvidenceReadInsertOpts())
	}); err != nil {
		f.t.Fatalf("queueing the first reading: %v", err)
	}
	f.read(args)
}

// rereads answers the queued readings that answer a removal.
func (f *rereadFixture) rereads() []StageEvidenceReadArgs {
	f.t.Helper()
	var out []StageEvidenceReadArgs
	for _, reading := range queuedJobs[StageEvidenceReadArgs](f) {
		if reading.RereadFor != nil {
			out = append(out, reading)
		}
	}
	return out
}

// sweep runs one page of the sweep with the given page size.
func (f *rereadFixture) sweep(args StageEvidenceRereadArgs, page int) {
	f.t.Helper()
	worker := newStageEvidenceRereadWorker(f.Pool, true, slog.New(slog.DiscardHandler))
	worker.Page = page
	if err := worker.Work(context.Background(), &river.Job[StageEvidenceRereadArgs]{Args: args}); err != nil {
		f.t.Fatalf("running the sweep: %v", err)
	}
}

// queued answers the args of this workspace's queued jobs of one kind, oldest
// first.
func queuedJobs[T river.JobArgs](f *rereadFixture) []T {
	f.t.Helper()
	var kind T
	rows, err := f.Pool.Query(context.Background(),
		`SELECT args FROM river_job WHERE kind = $1 AND args->>'workspace_id' = $2 ORDER BY id`,
		kind.Kind(), f.WS.String())
	if err != nil {
		f.t.Fatalf("reading queued %s jobs: %v", kind.Kind(), err)
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			f.t.Fatal(err)
		}
		var args T
		if err := json.Unmarshal(raw, &args); err != nil {
			f.t.Fatalf("decoding a queued %s job: %v", kind.Kind(), err)
		}
		out = append(out, args)
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *rereadFixture) evidenceOn(mail ids.UUID) int {
	f.t.Helper()
	return f.WsCount(f.t, `SELECT count(*) FROM deal_stage_evidence WHERE source_id = $1`, mail)
}

func TestMailFromARemovedOwnDomainIsReadAgain(t *testing.T) {
	f := newRereadFixture(t)
	if _, err := f.own.Add(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	fromListed := f.mail("cfo@buyer-corp.test")
	fromOther := f.mail("ops@other-customer.test")
	archived := f.mail("legal@buyer-corp.test")
	f.firstReading(fromListed)
	f.firstReading(fromOther)
	f.firstReading(archived)
	f.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)
	if got := f.evidenceOn(fromListed); got != 0 {
		t.Fatalf("mail from an own domain left %d evidence rows, want none: the ledger refuses our own word", got)
	}
	if got := f.evidenceOn(fromOther); got != 1 {
		t.Fatalf("mail from a customer left %d evidence rows, want 1", got)
	}

	if err := f.own.Remove(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	sweeps := queuedJobs[StageEvidenceRereadArgs](f)
	if len(sweeps) != 1 || sweeps[0].Domain != "buyer-corp.test" || sweeps[0].Removal.IsZero() || !sweeps[0].After.IsZero() {
		t.Fatalf("the removal queued %+v, want one sweep of buyer-corp.test from the start", sweeps)
	}
	f.sweep(sweeps[0], 0)
	// The first reading's row is still in the table, and the second is queued
	// beside it instead of coalescing with it.
	readings := f.rereads()
	if len(readings) != 1 || readings[0].ActivityID != fromListed || readings[0].DealID != f.dealID.UUID {
		t.Fatalf("the sweep queued %+v, want one reading of the live mail the removed domain sent", readings)
	}
	if *readings[0].RereadFor != sweeps[0].Removal {
		t.Errorf("the reading answers %s, want the removal %s", readings[0].RereadFor, sweeps[0].Removal)
	}
	f.read(readings[0])
	if got := f.WsScalar(t, `SELECT author_side FROM deal_stage_evidence WHERE source_id = $1`, fromListed); got != "buyer" {
		t.Errorf("the reread mail's evidence is %s-authored, want buyer", got)
	}

	// A sweep and a reading that River runs twice change nothing.
	asked := f.lane.calls
	f.sweep(sweeps[0], 0)
	if again := f.rereads(); len(again) != 1 {
		t.Errorf("a repeated sweep left %d queued readings, want the one it coalesces with", len(again))
	}
	f.read(readings[0])
	if got := f.evidenceOn(fromListed); got != 1 {
		t.Errorf("a repeated reading left %d evidence rows, want 1", got)
	}
	if got := f.evidenceOn(fromOther) + f.evidenceOn(archived); got != 1 {
		t.Errorf("mail the sweep must not read holds %d evidence rows, want the customer's 1", got)
	}
	if f.lane.calls != asked+1 {
		t.Errorf("the model was asked %d times after the first pass, want 1: the repeated reading alone", f.lane.calls-asked)
	}
}

func TestTheSweepPagesThroughADomainsMail(t *testing.T) {
	f := newRereadFixture(t)
	if _, err := f.own.Add(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	sent := map[ids.UUID]bool{f.mail("cfo@buyer-corp.test"): true, f.mail("cto@mail.buyer-corp.test"): true}
	if err := f.own.Remove(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	// Each page holds one activity. A full page queues the next, and the
	// empty page after the last queues nothing.
	for page := range 3 {
		sweeps := queuedJobs[StageEvidenceRereadArgs](f)
		if len(sweeps) != page+1 {
			t.Fatalf("after %d pages %d sweep jobs are queued, want %d", page, len(sweeps), page+1)
		}
		f.sweep(sweeps[page], 1)
	}
	if sweeps := queuedJobs[StageEvidenceRereadArgs](f); len(sweeps) != 3 {
		t.Errorf("the page after the last queued another: %d sweep jobs, want 3", len(sweeps))
	}
	readings := queuedJobs[StageEvidenceReadArgs](f)
	if len(readings) != 2 {
		t.Fatalf("two pages queued %d readings, want 2", len(readings))
	}
	for _, reading := range readings {
		if !sent[reading.ActivityID] {
			t.Errorf("a reading names activity %s, which the domain did not send", reading.ActivityID)
		}
		delete(sent, reading.ActivityID)
	}
}

func TestADomainListedAgainSpendsNoReading(t *testing.T) {
	f := newRereadFixture(t)
	if _, err := f.own.Add(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	f.mail("cfo@buyer-corp.test")
	if err := f.own.Remove(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.own.Add(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	f.sweep(queuedJobs[StageEvidenceRereadArgs](f)[0], 0)
	if readings := queuedJobs[StageEvidenceReadArgs](f); len(readings) != 0 {
		t.Errorf("the sweep queued %d readings of mail that is ours again; each would be refused", len(readings))
	}
}

func TestWithNoModelLaneTheSweepQueuesNothing(t *testing.T) {
	f := newRereadFixture(t)
	f.mail("cfo@buyer-corp.test")
	worker := newStageEvidenceRereadWorker(f.Pool, false, slog.New(slog.DiscardHandler))
	args := StageEvidenceRereadArgs{Workspace: f.WS, Domain: "buyer-corp.test", Removal: ids.NewV7()}
	if err := worker.Work(context.Background(), &river.Job[StageEvidenceRereadArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	if readings := queuedJobs[StageEvidenceReadArgs](f); len(readings) != 0 {
		t.Errorf("a role with no model lane queued %d readings no worker would run", len(readings))
	}
}

func TestRemovingACandidateNobodyConfirmedQueuesNoSweep(t *testing.T) {
	f := newRereadFixture(t)
	f.WsExec(t, `INSERT INTO workspace_email_domain (domain) VALUES ('seen-once.test')`)
	if err := f.own.Remove(f.Admin(), "seen-once.test"); err != nil {
		t.Fatal(err)
	}
	if got := f.WsCount(t, `SELECT count(*) FROM workspace_email_domain WHERE domain = 'seen-once.test'`); got != 0 {
		t.Fatalf("the candidate is still listed")
	}
	if sweeps := queuedJobs[StageEvidenceRereadArgs](f); len(sweeps) != 0 {
		t.Errorf("removing a domain that never counted as ours queued %d sweeps", len(sweeps))
	}
}

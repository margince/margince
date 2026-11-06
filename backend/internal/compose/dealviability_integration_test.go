// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A deal whose customer went silent half a year ago, with a close date three
// months ahead, is what the close-date check missed. Its date is in the future,
// so nothing about it looked wrong. These tests drive the real worker over
// deals written through the deals and activities stores, and read the receipt
// the check stores for each deal.

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type viabilityEnv struct {
	*integration.Env
	worker    *assuranceSweepWorker
	now       time.Time
	closeDate time.Time
	silent    ids.UUID
	supported ids.UUID
	buyer     ids.UUID
}

func setupViability(t *testing.T) *viabilityEnv {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	now := clocktest.Now(t)
	v := &viabilityEnv{
		Env: e, now: now, buyer: e.SeedContact(t, "Buyer", nil),
		closeDate: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 94),
		worker: &assuranceSweepWorker{
			pool: e.Pool, now: func() time.Time { return now }, log: slog.New(slog.DiscardHandler),
			activities: activities.NewStore(InstallationDB(e.Pool)),
		},
	}
	v.readMailboxOnTime(t)
	v.silent = v.commitDeal(t, pipeline, open, "Silent since spring", 500_000, now.AddDate(0, 0, -196))
	v.supported = v.commitDeal(t, pipeline, open, "Answered last week", 100_000, now.AddDate(0, 0, -5))
	return v
}

// readMailboxOnTime is a mail connector that synced three hours ago, so the
// check may observe silence. Without it the deal would read unknown.
func (v *viabilityEnv) readMailboxOnTime(t *testing.T) {
	t.Helper()
	v.mailboxReadOnTime(t, v.Rep1)
}

// mailboxReadOnTime gives user a mail connector that synced three hours ago. A
// deal is judged on its owner's mailboxes, so a new owner needs one too.
func (v *viabilityEnv) mailboxReadOnTime(t *testing.T, user ids.UUID) {
	t.Helper()
	owner := integration.OwnerConn(t)
	conn := integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'gmail', $2, 'connected')`, user)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO capture_sync_state (connection_id, last_success_at)
		VALUES ($1, now() - interval '3 hours')`, conn); err != nil {
		t.Fatal(err)
	}
}

// commitDeal writes a committed deal with a confirmed close date and the
// customer's last mail at lastReply.
func (v *viabilityEnv) commitDeal(
	t *testing.T, pipeline ids.PipelineID, stage ids.StageID, name string, amount int64, lastReply time.Time,
) ids.UUID {
	t.Helper()
	owner := ids.From[ids.UserKind](v.Rep1)
	currency := "EUR"
	created, err := v.Deals.CreateDeal(v.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: stage, OwnerID: &owner,
		AmountMinor: &amount, Currency: &currency, ExpectedClose: &v.closeDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	deal := ids.UUID(created.Id)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		WinProbability: new(95),
	}); err != nil {
		t.Fatal(err)
	}
	v.customerWrites(t, deal, lastReply)
	v.planNextStep(t, deal)
	return deal
}

// planNextStep writes the open task that is the deal's next step.
func (v *viabilityEnv) planNextStep(t *testing.T, deal ids.UUID) {
	t.Helper()
	subject, due := "Send the order form", v.now.AddDate(0, 0, 7)
	if _, _, err := v.Activities.LogActivity(v.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatal(err)
	}
}

// customerWrites logs a call with the buyer on the deal: the customer moving it.
func (v *viabilityEnv) customerWrites(t *testing.T, deal ids.UUID, at time.Time) {
	t.Helper()
	subject := "Call with the buyer"
	if _, _, err := v.Activities.LogActivity(v.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "deal", EntityID: deal}, {EntityType: "contact", EntityID: v.buyer},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

// check runs the input check.
func (v *viabilityEnv) check(t *testing.T) {
	t.Helper()
	worker := &assuranceRunWorker{sweep: v.worker}
	if err := worker.Work(context.Background(), &river.Job[AssuranceRunArgs]{
		Args: AssuranceRunArgs{Workspace: v.WS, RequestedBy: v.Rep1.String()},
	}); err != nil {
		t.Fatalf("the input check: %v", err)
	}
}

// receiptStale answers whether the deal's evidence moved since the check
// stored its receipt, by the fingerprint the freshness check compares.
func (v *viabilityEnv) receiptStale(t *testing.T, deal ids.UUID) bool {
	t.Helper()
	return v.WsScalar(t, `SELECT (r.input_fingerprint IS DISTINCT FROM `+viabilityFingerprintSQL("d")+`)::text
		  FROM deal d JOIN assurance_deal_receipt r ON r.deal_id = d.id WHERE d.id = $1`, deal) == "true"
}

func TestEvidenceThatMovedSinceTheCheckMakesTheReceiptStale(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	if v.receiptStale(t, v.supported) {
		t.Fatal("the receipt is stale right after the check")
	}
	// The supported deal's customer writes again after the check. The receipt
	// describes evidence the deal no longer carries.
	v.customerWrites(t, v.supported, v.now.Add(-time.Minute))
	if !v.receiptStale(t, v.supported) {
		t.Fatal("a new customer call left the receipt current")
	}
}

func hasReason(list, item string) bool {
	return slices.Contains(strings.Split(list, ","), item)
}

func TestABrokenColleagueMailboxDoesNotBlindTheOwnersDeals(t *testing.T) {
	v := setupViability(t)
	// The fixture's pipeline and stage, read off a deal it already made.
	var pipeline ids.PipelineID
	var open ids.StageID
	if err := v.Pool.QueryRow(context.Background(), `SELECT pipeline_id, stage_id FROM deal WHERE id = $1`,
		v.silent).Scan(&pipeline, &open); err != nil {
		t.Fatal(err)
	}
	integration.SeedIDRow(t, integration.OwnerConn(t), `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'imap', $2, 'reauth_required')`, v.Rep2)
	theirs := v.commitDeal(t, pipeline, open, "Colleague's quiet deal", 50_000, v.now.AddDate(0, 0, -196))
	colleague := ids.From[ids.UserKind](v.Rep2)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](theirs), deals.UpdateDealInput{
		OwnerID: &colleague,
	}); err != nil {
		t.Fatal(err)
	}

	v.check(t)

	if outcome, _ := v.receiptOf(t, v.silent); outcome != "review_required" {
		t.Fatalf("the silent deal whose owner's mailbox was read reads %s, want review_required", outcome)
	}
	if got := v.WsScalar(t, `SELECT outcome || ':' || array_to_string(reasons, ',')
		FROM assurance_deal_receipt WHERE deal_id = $1`, theirs); got != "unknown:coverage_unknown" {
		t.Fatalf("the colleague's deal reads %q, want unknown:coverage_unknown", got)
	}
	if got := v.WsScalar(t, `SELECT status FROM assurance_run ORDER BY created_at DESC, id DESC LIMIT 1`); got != assurance.StatusIncomplete {
		t.Fatalf("the run reads %q, want %q: a broken mailbox still makes the run incomplete", got, assurance.StatusIncomplete)
	}
}

// A DEAL WHOSE MAIL REACHED A BROKEN MAILBOX STAYS UNKNOWN. The owner's mailbox
// was read on time, but a colleague's mailbox captured mail on the deal and
// needs re-authorising. A customer answer may sit unread in that mailbox, so
// the deal's silence is not observed.
//
// Mutation: drop the capture_import users from dealMailCovered (judge the owner
// alone) and the silent deal reads review_required.
func TestADealWhoseMailReachedABrokenMailboxStaysUnknown(t *testing.T) {
	v := setupViability(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'imap', $2, 'reauth_required')`, v.Rep2)
	// Our own earlier mail on the deal, captured by the colleague's mailbox.
	// Ours, so it is no customer progress and the deal is still silent.
	mail := integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', now() - interval '200 days', 'imap', 'connector:imap:' || $2::text)`, v.Rep2)
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, mail, v.silent); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, mail, v.Rep2); err != nil {
		t.Fatal(err)
	}

	v.check(t)

	if got := v.WsScalar(t, `SELECT outcome || ':' || array_to_string(reasons, ',')
		FROM assurance_deal_receipt WHERE deal_id = $1`, v.silent); got != "unknown:coverage_unknown" {
		t.Fatalf("the silent deal reads %q, want unknown:coverage_unknown", got)
	}
}

// reassign gives a deal to another owner.
func (v *viabilityEnv) reassign(t *testing.T, deal, owner ids.UUID) {
	t.Helper()
	to := ids.From[ids.UserKind](owner)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &to}); err != nil {
		t.Fatal(err)
	}
}

// ourMailOnDeal links a message we sent long ago to the deal, delivered to
// Margince by the named connections of a colleague's mailbox. Ours, so it is no
// customer progress and the deal stays silent.
func (v *viabilityEnv) ourMailOnDeal(t *testing.T, deal, importer ids.UUID, deliveredBy ...string) {
	t.Helper()
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	mail := integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', $2, 'imap', 'connector:imap:' || $3::text)`,
		v.now.AddDate(0, 0, -200), importer)
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, mail, deal); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO capture_import (activity_id, user_id, delivered_by) VALUES ($1, $2, $3)`,
		mail, importer, deliveredBy); err != nil {
		t.Fatal(err)
	}
}

// A healthy run does not cover a deal whose owner has no mailbox: nothing
// shows the owner's mail was read at all.
//
// Mutation: let a checked run grade cover every deal again in dealMailCovered
// and the deal reads review_required.
func TestAHealthyRunDoesNotCoverADealWhoseOwnerHasNoMailbox(t *testing.T) {
	v := setupViability(t)
	v.reassign(t, v.silent, v.Rep2)

	v.check(t)

	if outcome, reasons := v.receiptOf(t, v.silent); outcome != "unknown" || reasons != "coverage_unknown" {
		t.Fatalf("the deal reads %s:%s, want unknown:coverage_unknown", outcome, reasons)
	}
}

// The deal's mail came through a colleague's Gmail connection, which is gone.
// The colleague's new, healthy Outlook connection never held that mail, so it
// does not cover the deal.
//
// Mutation: judge an import on its user's connections and ignore delivered_by,
// and the deal reads review_required.
func TestAGoneDeliveringConnectionIsNotCoveredByTheUsersNewOne(t *testing.T) {
	v := setupViability(t)
	owner := integration.OwnerConn(t)
	graph := integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'graph', $2, 'connected')`, v.Rep2)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO capture_sync_state (connection_id, last_success_at)
		VALUES ($1, now() - interval '1 hour')`, graph); err != nil {
		t.Fatal(err)
	}
	v.ourMailOnDeal(t, v.silent, v.Rep2, "gmail:0")

	v.check(t)

	if outcome, reasons := v.receiptOf(t, v.silent); outcome != "unknown" || reasons != "coverage_unknown" {
		t.Fatalf("the deal reads %s:%s, want unknown:coverage_unknown", outcome, reasons)
	}
}

// A mailbox that delivers the deal's mail after the check changes what the
// deal's coverage rests on, so the receipt no longer counts. Nothing else about
// the deal changed: the message was already on it.
//
// Mutation: drop the mailboxes from viabilityFingerprintSQL and the receipt
// stays current.
func TestANewDeliveringMailboxMakesTheReceiptStale(t *testing.T) {
	v := setupViability(t)
	v.ourMailOnDeal(t, v.supported, v.Rep1, "gmail:0")
	v.check(t)
	if v.receiptStale(t, v.supported) {
		t.Fatal("the receipt is stale right after the check")
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO capture_import (activity_id, user_id, delivered_by)
		SELECT ci.activity_id, $2, '{imap:0}' FROM capture_import ci
		  JOIN activity_link l ON l.activity_id = ci.activity_id
		 WHERE l.deal_id = $1`, v.supported, v.Rep2); err != nil {
		t.Fatal(err)
	}

	if !v.receiptStale(t, v.supported) {
		t.Fatal("a second mailbox delivering the deal's mail left the receipt current")
	}
}

// A past meeting with the buyer moves the deal only once someone recorded it
// held. One with no status, or still booked, is awaiting its outcome, so the
// customer-progress clock does not start from it.
func TestOnlyAHeldMeetingIsCustomerProgress(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	buyer, now := e.SeedContact(t, "Buyer", nil), clocktest.Now(t)
	progressOf := func(deal ids.UUID) string {
		return e.WsScalar(t, `SELECT coalesce(max(a.occurred_at)::text, 'none') FROM activity a
			JOIN activity_link al ON al.activity_id = a.id
			WHERE al.deal_id = $1 AND a.archived_at IS NULL AND `+customerProgressSQL("a", "$2"), deal, now)
	}
	for _, status := range []string{"", "booked", "held"} {
		deal := e.SeedDeal(t, "Met the buyer ("+status+")", pipeline, open, &e.Rep1)
		subject, at := "Workshop with the buyer", now.AddDate(0, 0, -3)
		in := activities.LogActivityInput{
			Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}, {EntityType: "contact", EntityID: buyer}},
		}
		if status != "" {
			in.MeetingStatus = &status
		}
		if _, _, err := e.Activities.LogActivity(e.Admin(), in); err != nil {
			t.Fatal(err)
		}
		if got, moved := progressOf(deal), status == "held"; (got != "none") != moved {
			t.Errorf("a past meeting with status %q: customer progress at %s, want progress %v", status, got, moved)
		}
	}
}

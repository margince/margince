// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newsletterBody is a plain-text message with the List-Unsubscribe header a
// mailing tool adds. The fixture's mail appends it after its own headers.
func newsletterBody(text string) []string {
	return []string{
		"List-Unsubscribe: <mailto:leave@client.test>",
		"Content-Type: text/plain; charset=utf-8", "", text, "",
	}
}

// humanExchange captures a question from the contact and the seat's answer,
// and returns the reading they give.
func (f *relationshipFixture) humanExchange(t *testing.T) relationshipReading {
	t.Helper()
	asked := f.mail(t, "pricing-question", f.contactAddr, f.seatAddr, "", 20)
	answered := f.mail(t, "pricing-answer", f.seatAddr, f.contactAddr, "", 18)
	return relationshipReading{
		inbound90d: 1, outbound90d: 1,
		contactIn: instantOf(t, f.e, asked), contactOut: instantOf(t, f.e, answered),
		companyIn: instantOf(t, f.e, asked), companyOut: instantOf(t, f.e, answered),
		companyTouch: instantOf(t, f.e, answered), companyTouchID: ids.ActivityID{UUID: answered},
		edgeCount90d: 2,
	}
}

func requireBulkStamp(t *testing.T, f *relationshipFixture, activity ids.UUID) {
	t.Helper()
	if !workspaceValue[bool](t, f.e, `SELECT bulk_mail_attested FROM activity WHERE id = $1`, activity) {
		t.Fatal("capture did not stamp the List-Unsubscribe mail as bulk, so the readings below prove nothing")
	}
}

// A newsletter either way changes no reading of the relationship, and still
// shows on every timeline marked as bulk. A reply without the header moves
// every reading.
func TestANewsletterIsNoExchangeButStaysOnTheTimeline(t *testing.T) {
	f := newRelationshipFixture(t)
	before := f.humanExchange(t)
	assertReading(t, "two humans writing", f.read(t), before)

	theirs := f.mail(t, "client-newsletter", f.contactAddr, f.seatAddr, "", 4, newsletterBody("Our autumn newsletter.")...)
	ours := f.mail(t, "our-newsletter", f.seatAddr, f.contactAddr, "", 3, newsletterBody("News from us.")...)
	requireBulkStamp(t, f, theirs)
	requireBulkStamp(t, f, ours)
	assertReading(t, "after two newsletters", f.read(t), before)

	reply := f.mail(t, "pricing-followup", f.contactAddr, f.seatAddr, "", 1)
	after := before
	after.inbound90d = 2
	after.contactIn, after.companyIn = instantOf(t, f.e, reply), instantOf(t, f.e, reply)
	after.companyTouch, after.companyTouchID = instantOf(t, f.e, reply), ids.ActivityID{UUID: reply}
	after.edgeCount90d = 3
	assertReading(t, "after their reply", f.read(t), after)

	contact := "contact"
	listed, _, err := f.e.Activities.ListActivities(f.e.Admin(), activities.ListActivitiesInput{EntityType: &contact, EntityID: &f.contact})
	if err != nil {
		t.Fatal(err)
	}
	assertMarkedBulk(t, "the activity list", listed, theirs, ours)

	clock := func() time.Time { return f.now }
	page, err := contact360.NewService(f.e.Pool, f.e.Contacts, f.e.Deals, f.e.Projects,
		consent.NewStore(InstallationDB(f.e.Pool)),
		comms.NewStore(InstallationDB(f.e.Pool), clock, activities.NewStore(InstallationDB(f.e.Pool))),
		ai.NewFeedbackStore(InstallationDB(f.e.Pool)), clock).Assemble(f.e.Admin(), ids.From[ids.ContactKind](f.contact))
	if err != nil {
		t.Fatal(err)
	}
	if page.Activities == nil {
		t.Fatal("the contact page has no timeline")
	}
	assertMarkedBulk(t, "the contact page", page.Activities.Data, theirs, ours)
}

func assertMarkedBulk(t *testing.T, surface string, rows []crmcontracts.Activity, want ...ids.UUID) {
	t.Helper()
	kinds := map[ids.UUID]*crmcontracts.ActivityAutomaticKind{}
	for _, row := range rows {
		kinds[ids.UUID(row.Id)] = row.AutomaticKind
	}
	for _, id := range want {
		kind, shown := kinds[id]
		switch {
		case !shown:
			t.Errorf("%s leaves out the newsletter %s", surface, id)
		case kind == nil || *kind != crmcontracts.ActivityAutomaticKindBulk:
			t.Errorf("%s shows the newsletter %s with automatic_kind %v, want bulk", surface, id, kind)
		}
	}
}

// An edge folded while a newsletter still counted heals on the nightly
// rebuild, with no data task.
func TestTheNightlyRebuildDropsANewslettersEdgeCount(t *testing.T) {
	f := newRelationshipFixture(t)
	f.humanExchange(t)
	newsletter := f.mail(t, "client-newsletter", f.contactAddr, f.seatAddr, "", 4, newsletterBody("Our autumn newsletter.")...)
	requireBulkStamp(t, f, newsletter)

	f.e.WsExec(t, `UPDATE activity SET bulk_mail_attested = false WHERE id = $1`, newsletter)
	if got := f.read(t).edgeCount90d; got != 3 {
		t.Fatalf("an unstamped newsletter folds into %d exchanges, want 3, so the repair below proves nothing", got)
	}
	f.e.WsExec(t, `UPDATE activity SET bulk_mail_attested = true WHERE id = $1`, newsletter)

	if err := newGraphEdgeReconcileWorker(f.e.Pool, quietLog()).reconcileWorkspace(context.Background(), f.e.WS); err != nil {
		t.Fatalf("reconcile pass: %v", err)
	}
	var edge int
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT coalesce(max(count_90d), 0) FROM graph_interaction_edge
			 WHERE user_id = $1 AND contact_id = $2`, f.seat, f.contact).Scan(&edge)
	}); err != nil {
		t.Fatal(err)
	}
	if edge != 2 {
		t.Errorf("after the nightly rebuild the edge counts %d exchanges, want the 2 humans wrote", edge)
	}
}

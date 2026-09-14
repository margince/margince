// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Deal.last_email: the newest email the whole workspace may see on a deal,
// carried by the list the board draws and by the single read the record page
// makes. A note newer than every mail does not move it, because it is about
// MAIL; a mail limited to its participants does not move it, because a
// colleague outside that audience must not read its date off a card; archiving
// the newest mail hands the reading to the next one, because it is computed
// from the live timeline rather than kept as a high-water mark.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDealLastEmail_TheNewestWorkspaceMailAndOnlyThat(t *testing.T) {
	e := Setup(t)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	mailed, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Mailed deal", AmountMinor: int64Ptr(100), Currency: strPtr("EUR"),
		PipelineID: pipeline, StageID: open, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Quiet deal", AmountMinor: int64Ptr(100), Currency: strPtr("EUR"),
		PipelineID: pipeline, StageID: open, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	mailedID := ids.From[ids.DealKind](ids.UUID(mailed.Id))
	link := activities.ActivityLinkInput{EntityType: "deal", EntityID: ids.UUID(mailed.Id)}
	log := func(kind, direction string, when time.Time) ids.UUID {
		t.Helper()
		subject := "about the deal"
		in := activities.LogActivityInput{
			Kind: kind, Subject: &subject, OccurredAt: &when, Source: "manual",
			Links: []activities.ActivityLinkInput{link},
		}
		if direction != "" {
			in.Direction = &direction
		}
		logged, _, err := e.Activities.LogActivity(e.Admin(), in)
		if err != nil {
			t.Fatalf("logging a %s: %v", kind, err)
		}
		return ids.UUID(logged.Id)
	}

	sent := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	replied := sent.Add(48 * time.Hour)
	noted := replied.Add(24 * time.Hour)
	held := noted.Add(24 * time.Hour)
	log("email", "outbound", sent)
	reply := log("email", "inbound", replied)
	// Newer than every mail, and not mail: the reading is about the exchange
	// by email, and a note a colleague typed is not one.
	log("note", "", noted)
	// Newest of all, and held to its participants: an admin who is not on it
	// must not learn its date from the card.
	heldMail := log("email", "inbound", held)
	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](heldMail),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("narrowing the newest mail: %v", err)
	}

	expectLast := func(where string, got *crmcontracts.DealLastEmail, want time.Time, direction crmcontracts.DealLastEmailDirection) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: last_email is null, want %v %s", where, want, direction)
		}
		if !got.OccurredAt.Equal(want) {
			t.Errorf("%s: last_email.occurred_at = %v, want %v", where, got.OccurredAt, want)
		}
		if got.Direction == nil || *got.Direction != direction {
			t.Errorf("%s: last_email.direction = %v, want %s", where, got.Direction, direction)
		}
	}

	// The list — what the board draws — and the single read agree, and both
	// point at the reply: the newest mail everybody may see.
	listed := listDealsByID(t, e)
	expectLast("list", listed[ids.UUID(mailed.Id)].LastEmail, replied, crmcontracts.DealLastEmailDirectionInbound)
	if got := listed[ids.UUID(quiet.Id)].LastEmail; got != nil {
		t.Errorf("a deal nobody mailed about carries last_email %+v, want null", got)
	}
	single, err := e.Deals.GetDeal(e.Admin(), mailedID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	expectLast("single read", single.LastEmail, replied, crmcontracts.DealLastEmailDirectionInbound)

	// Archiving the reply hands the reading to the mail before it, direction
	// and all: a recompute from what is live, never a mark that outlives it.
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](reply), nil); err != nil {
		t.Fatal(err)
	}
	single, err = e.Deals.GetDeal(e.Admin(), mailedID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	expectLast("after archiving the newest mail", single.LastEmail, sent, crmcontracts.DealLastEmailDirectionOutbound)
}

// listDealsByID reads every deal the admin sees, keyed by id, so a case asks
// about the row it seeded rather than about a position in the page.
func listDealsByID(t *testing.T, e *Env) map[ids.UUID]crmcontracts.Deal {
	t.Helper()
	rows, _, err := e.Deals.ListDeals(e.Admin(), deals.ListDealsInput{})
	if err != nil {
		t.Fatalf("listing deals: %v", err)
	}
	out := make(map[ids.UUID]crmcontracts.Deal, len(rows))
	for _, row := range rows {
		out[ids.UUID(row.Id)] = row
	}
	return out
}

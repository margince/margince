// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// justInLater is an hour after the fixtures' mail, well inside the window.
var justInLater = requestInstant.Add(time.Hour)

func latestOn(t *testing.T, e *loadEnv, store *Store, contact ids.UUID) []crmcontracts.LatestInbound {
	t.Helper()
	list, err := store.LatestInbound(e.asSeat(e.rep), "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	return list.Data
}

func TestLatestInboundOffersAcceptingARequestUntilItsTaskStands(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	source := seedEmailRequestFiledUnder(t, e, "Send the revised offer", "commitment", OwedVerdictAsksUs, requestCounterparty, links, mailParticipants{})
	store := storeKnowing(e).WithClock(func() time.Time { return justInLater })

	items := latestOn(t, e, store, contact)
	if len(items) != 1 || ids.UUID(items[0].Activity.Id) != source {
		t.Fatalf("items = %+v, want the waiting request", items)
	}
	if items[0].Kind != crmcontracts.LatestInboundKindEmail || !items[0].AcceptsRequest {
		t.Fatalf("item = kind %s accepts %v, want an email whose request a task accepts", items[0].Kind, items[0].AcceptsRequest)
	}
	if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source}); err != nil {
		t.Fatalf("the request the read offered was refused: %v", err)
	}
	if items := latestOn(t, e, store, contact); len(items) != 0 {
		t.Fatalf("items = %+v, want the request its task now stands for left out", items)
	}
}

func TestLatestInboundShowsAWebsiteFormUntilACallAnswersIt(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	store := storeKnowing(e).WithClock(func() time.Time { return justInLater })
	subject, system, key := "Website form: Contact us", provenance.WebFormSource, ids.NewV7().String()
	form, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
		Kind: "note", Subject: &subject, OccurredAt: &requestInstant, Links: links,
		SourceSystem: &system, SourceID: &key, Source: provenance.WebFormSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	items := latestOn(t, e, store, contact)
	if len(items) != 1 || items[0].Activity.Id != form.Id || items[0].Kind != crmcontracts.LatestInboundKindWebForm {
		t.Fatalf("items = %+v, want the open form", items)
	}
	if items[0].AcceptsRequest {
		t.Fatal("a form is no request; a task made from it carries source_activity_id")
	}
	called, note := requestInstant.Add(30*time.Minute), "Called back about the form"
	if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "call", Source: "manual", Subject: &note, OccurredAt: &called, Links: links}); err != nil {
		t.Fatal(err)
	}
	if items := latestOn(t, e, store, contact); len(items) != 0 {
		t.Fatalf("items = %+v, want the form answered by the call", items)
	}
}

func TestLatestInboundLeavesOutAFormATaskWasMadeFrom(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	store := storeKnowing(e).WithClock(func() time.Time { return justInLater })
	subject, system, key := "Website form: Demo", provenance.WebFormSource, ids.NewV7().String()
	form, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
		Kind: "note", Subject: &subject, OccurredAt: &requestInstant, Links: links,
		SourceSystem: &system, SourceID: &key, Source: provenance.WebFormSource,
	})
	if err != nil {
		t.Fatal(err)
	}
	formID, task := ids.UUID(form.Id), "Book the demo"
	if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", Subject: &task, SourceActivityID: &formID, Links: links}); err != nil {
		t.Fatal(err)
	}
	if items := latestOn(t, e, store, contact); len(items) != 0 {
		t.Fatalf("items = %+v, want the form its task stands for left out", items)
	}
}

func TestLatestInboundLeavesOutWhatArrivedBeforeTheWindow(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	seedEmailRequestFiledUnder(t, e, "An old ask", "commitment", OwedVerdictAsksUs, requestCounterparty, links, mailParticipants{})
	store := storeKnowing(e).WithClock(func() time.Time { return requestInstant.AddDate(0, 0, JustInDays+1) })
	if items := latestOn(t, e, store, contact); len(items) != 0 {
		t.Fatalf("items = %+v, want nothing older than %d days", items, JustInDays)
	}
}

func TestLatestInboundSaysWhoseMoveTheLastExchangeLeaves(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	store := storeKnowing(e).WithClock(func() time.Time { return requestInstant.AddDate(0, 0, JustInDays+1) })
	exchange := func() *crmcontracts.LastExchange {
		t.Helper()
		list, err := store.LatestInbound(e.asSeat(e.rep), "contact", contact)
		if err != nil {
			t.Fatal(err)
		}
		return list.LastExchange
	}
	if got := exchange(); got != nil {
		t.Fatalf("last exchange = %+v, want none on a record nobody spoke with", got)
	}
	source := seedEmailRequestFiledUnder(t, e, "Send the offer", "commitment", OwedVerdictAsksUs, requestCounterparty, links, mailParticipants{})
	got := exchange()
	if got == nil || ids.UUID(got.Activity.Id) != source || got.WhoseMove != crmcontracts.LastExchangeWhoseMoveUs {
		t.Fatalf("last exchange = %+v, want their ask, older than Just in, owed by us", got)
	}
	called, note, out := requestInstant.Add(time.Hour), "Left a voicemail", string(crmcontracts.ActivityDirectionOutbound)
	if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "call", Source: "manual", Subject: &note, OccurredAt: &called, Direction: &out, Links: links}); err != nil {
		t.Fatal(err)
	}
	if got := exchange(); got == nil || got.WhoseMove != crmcontracts.LastExchangeWhoseMoveThem {
		t.Fatalf("last exchange = %+v, want our call waiting on them", got)
	}
}

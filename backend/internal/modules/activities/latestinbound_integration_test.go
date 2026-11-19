// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

func TestLatestInboundKeepsAFormUntilARowThatAnswersIt(t *testing.T) {
	out, in := string(crmcontracts.ActivityDirectionOutbound), string(crmcontracts.ActivityDirectionInbound)
	status := func(s crmcontracts.ActivityMeetingStatus) *string { v := string(s); return &v }
	cases := []struct {
		name    string
		kind    string
		dir     *string
		meeting *string
		after   time.Duration
		answers bool
	}{
		{"a sent mail", "email", &out, nil, 30 * time.Minute, true},
		{"a call", "call", nil, nil, 30 * time.Minute, true},
		{"a held meeting", "meeting", nil, status(crmcontracts.ActivityMeetingStatusHeld), 30 * time.Minute, true},
		{"a past meeting nobody reported", "meeting", nil, nil, 30 * time.Minute, true},
		{"a booked meeting still to come", "meeting", nil, status(crmcontracts.ActivityMeetingStatusBooked), 2 * time.Hour, false},
		{"their reply", "email", &in, nil, 30 * time.Minute, false},
		{"our own note", "note", nil, nil, 30 * time.Minute, false},
		{"a canceled meeting", "meeting", nil, status(crmcontracts.ActivityMeetingStatusCanceled), 30 * time.Minute, false},
		{"a no-show", "meeting", nil, status(crmcontracts.ActivityMeetingStatusNoShow), 30 * time.Minute, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setupLoad(t)
			contact := e.buyer(t)
			links := []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
			store := storeKnowing(e).WithClock(func() time.Time { return justInLater })
			subject, system, key := "Website form: Pricing", provenance.WebFormSource, ids.NewV7().String()
			if _, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
				Kind: "note", Subject: &subject, OccurredAt: &requestInstant, Links: links,
				SourceSystem: &system, SourceID: &key, Source: provenance.WebFormSource,
			}); err != nil {
				t.Fatal(err)
			}
			// The store's clock is justInLater, so a meeting two hours after the
			// form is still to come for it. The database clock is long past it.
			later, what := requestInstant.Add(c.after), "After the form"
			if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{
				Kind: c.kind, Source: "manual", Subject: &what, OccurredAt: &later,
				Direction: c.dir, MeetingStatus: c.meeting, Links: links,
			}); err != nil {
				t.Fatal(err)
			}
			open := false
			for _, item := range latestOn(t, e, store, contact) {
				open = open || item.Kind == crmcontracts.LatestInboundKindWebForm
			}
			if open == c.answers {
				t.Fatalf("form open = %v after %s, want answered = %v", open, c.name, c.answers)
			}
		})
	}
}

// A form and our answer at one stored instant: a date on either side makes ours its answer. Between
// two answers of ours at that instant, the day one answers whichever was logged last.
func TestLatestInboundReadsAFormTieByTheAnswerRule(t *testing.T) {
	out := string(crmcontracts.ActivityDirectionOutbound)
	for _, c := range []struct {
		name    string
		replies []string
		open    bool
	}{
		{"a day reply on the form's date", []string{PrecisionDay}, false},
		{"an instant reply at the form's instant", []string{PrecisionInstant}, true},
		{"our day reply, then our instant reply", []string{PrecisionDay, PrecisionInstant}, false},
		{"our instant reply, then our day reply", []string{PrecisionInstant, PrecisionDay}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := setupLoad(t)
			contact := e.buyer(t)
			links := []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
			store := storeKnowing(e).WithClock(func() time.Time { return justInLater })
			subject, system, key := "Website form: Trial", provenance.WebFormSource, ids.NewV7().String()
			if _, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
				Kind: "note", Subject: &subject, OccurredAt: &requestInstant, Links: links,
				SourceSystem: &system, SourceID: &key, Source: provenance.WebFormSource,
			}); err != nil {
				t.Fatal(err)
			}
			for _, precision := range c.replies {
				reply := "Re: your trial"
				if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{
					Kind: "email", Source: "manual", Subject: &reply, Direction: &out,
					OccurredAt: &requestInstant, OccurredPrecision: precision, Links: links,
				}); err != nil {
					t.Fatal(err)
				}
			}
			open := false
			for _, item := range latestOn(t, e, store, contact) {
				open = open || item.Kind == crmcontracts.LatestInboundKindWebForm
			}
			if open != c.open {
				t.Fatalf("form open = %v, want %v", open, c.open)
			}
		})
	}
}

func TestLatestInboundRefusesARecordNobodyHolds(t *testing.T) {
	e := setupLoad(t)
	_, err := storeKnowing(e).LatestInbound(e.asSeat(e.rep), "contact", ids.NewV7())
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not found for an id no contact holds", err)
	}
}

func (e *recencyEnv) seedMail(t *testing.T, deal ids.UUID, direction string, at time.Time) ids.UUID {
	t.Helper()
	mail := ids.NewV7()
	e.exec(t, `INSERT INTO activity
		  (id, kind, subject, occurred_at, direction, audience, source, captured_by, origin)
		VALUES ($1, 'email', 'Terms', $2, $3, 'workspace', 'manual', 'human:rep', 'human')`,
		mail, at, direction)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), mail, deal)
	return mail
}

func TestLatestInboundDatesTheLastMessagesOverEveryRowNotOnePage(t *testing.T) {
	e := setupRecency(t)
	deal := e.seedQuietDeal(t, pinnedQuiet)
	inbound := e.seedMail(t, deal, "inbound", pinnedQuiet.AddDate(0, 0, 1))
	outbound := e.seedMail(t, deal, "outbound", pinnedQuiet.AddDate(0, 0, 2))
	for day := range 60 {
		e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, audience, source, captured_by, origin)
			VALUES ($1, 'note', 'Later note', $2, 'workspace', 'manual', 'human:rep', 'human')`,
			ids.NewV7(), pinnedQuiet.AddDate(0, 0, 3+day))
		e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
			SELECT $1, id, 'deal', $2 FROM activity WHERE subject = 'Later note' AND occurred_at = $3`,
			ids.NewV7(), deal, pinnedQuiet.AddDate(0, 0, 3+day))
	}
	now := pinnedQuiet.AddDate(0, 0, 100)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).WithClock(func() time.Time { return now })

	list, err := store.LatestInbound(e.as(), "deal", deal)
	if err != nil {
		t.Fatal(err)
	}
	if list.LastInbound == nil || ids.UUID(list.LastInbound.Id) != inbound {
		t.Errorf("last inbound = %+v, want the mail %s", list.LastInbound, inbound)
	}
	if list.LastOutbound == nil || ids.UUID(list.LastOutbound.Id) != outbound {
		t.Errorf("last outbound = %+v, want the mail %s", list.LastOutbound, outbound)
	}
}

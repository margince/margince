// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A search hit on an email carries the canonical email row, so the result
// looks like the same message the timeline shows rather than a second
// rendering of it. The row is admitted by the SAME content gate the hit is,
// which is what keeps a limited conversation out of search entirely — hit and
// summary alike.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// searchEmailStore is the search store wired the way compose wires it: with
// the activities reader behind an email hit's summary. A test that built the
// store bare would prove nothing about the surface a person searches.
func searchEmailStore(e *Env) *search.Store {
	return search.NewStore(e.DB()).WithEmailSummaries(activities.EmailSummariesByIDBatch)
}

func hitFor(page search.Page, id ids.UUID) *search.Hit {
	for i := range page.Hits {
		if page.Hits[i].ID == id {
			return &page.Hits[i]
		}
	}
	return nil
}

// An email hit carries the canonical row: the subject, a body preview and the
// access badge, all from the activities projection rather than from a second
// one built here.
func TestAnEmailSearchHitCarriesTheCanonicalRow(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	subject, body := "Rennsteig renewal terms", "The quote is attached, and it holds until Friday."
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)

	page, err := searchEmailStore(e).Search(author, search.Input{
		Query: "Rennsteig", Types: []string{"activity"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	hit := hitFor(page, id)
	if hit == nil {
		t.Fatalf("the author's own mail did not appear in search: %+v", page.Hits)
	}
	if hit.EmailSummary == nil {
		t.Fatal("an email hit carried no email_summary; the canonical row is what the client renders")
	}
	got := *hit.EmailSummary
	if got.Subject == nil || *got.Subject != subject {
		t.Errorf("summary subject = %v, want %q", got.Subject, subject)
	}
	if got.Preview == nil || *got.Preview == "" {
		t.Error("summary carried no preview; the row shows one and the hit must show the same one")
	}
	if got.DisplayStatus != crmcontracts.EmailAccessStatusTeam {
		t.Errorf("display_status = %q, want team for an unlimited mail", got.DisplayStatus)
	}
	if got.Direction == nil || *got.Direction != crmcontracts.EmailSummaryDirectionInbound {
		t.Errorf("summary direction = %v, want inbound", got.Direction)
	}
	if got.ActivityId != logged.Id {
		t.Errorf("summary names activity %v, want the hit's own %v", got.ActivityId, logged.Id)
	}
	if got.Version == 0 {
		t.Error("summary carried no version; the audience write needs it for If-Match")
	}
}

// The privacy behaviour search already had does not change. A limited
// conversation produces NO hit for a colleague outside its audience — not a
// withheld one — because the activity branch is content-gated. Moving it to
// the discover gate would leak what the body says through hit existence,
// score, ordering and the page boundary.
func TestAWithheldEmailProducesNoSearchHit(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	subject, body := "Rennsteig severance package", "the agreed figure is confidential"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)
	store := searchEmailStore(e)

	// Before limiting, the colleague finds it — which is what makes the
	// disappearance below the audience's doing rather than the query's.
	before, err := e.Activities.GetActivity(colleague, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("colleague reading before limiting: %v", err)
	}
	if before.Subject == nil {
		t.Fatal("the colleague could not read the mail before it was limited; the fixture proves nothing")
	}
	openPage, err := store.Search(colleague, search.Input{Query: "Rennsteig", Types: []string{"activity"}})
	if err != nil {
		t.Fatalf("search before limiting: %v", err)
	}
	if hitFor(openPage, id) == nil {
		t.Fatal("the colleague did not find an unlimited mail; the fixture proves nothing")
	}

	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](id),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	// A search over the SUBJECT and one over the BODY, because either matching
	// would disclose what the message says.
	for _, q := range []string{"Rennsteig", "severance", "confidential"} {
		page, err := store.Search(colleague, search.Input{Query: q, Types: []string{"activity"}})
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		if hit := hitFor(page, id); hit != nil {
			t.Errorf("a limited mail surfaced in a colleague's search for %q: %+v", q, *hit)
		}
	}

	// The author is in its audience and keeps finding it, whole.
	mine, err := store.Search(author, search.Input{Query: "Rennsteig", Types: []string{"activity"}})
	if err != nil {
		t.Fatalf("author search: %v", err)
	}
	hit := hitFor(mine, id)
	if hit == nil || hit.EmailSummary == nil {
		t.Fatalf("the author lost their own limited mail from search: %+v", mine.Hits)
	}
	if hit.EmailSummary.DisplayStatus != crmcontracts.EmailAccessStatusParticipants {
		t.Errorf("display_status = %q, want participants — the badge says the mail is limited",
			hit.EmailSummary.DisplayStatus)
	}
}

// A call, a note, a task and a meeting are activities too. Each keeps the
// generic hit it has always had, so a client branching on email_summary's
// presence renders them the way it did before.
func TestANonEmailActivityHitKeepsItsGenericTreatment(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)
	store := searchEmailStore(e)

	for _, kind := range []string{"call", "note", "task", "meeting"} {
		subject := "Rennsteig " + kind + " follow-up"
		body := "what we agreed on the " + kind
		logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
			Kind: kind, Subject: &subject, Body: &body,
			Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
		})
		if err != nil {
			t.Fatalf("log %s: %v", kind, err)
		}
		page, err := store.Search(author, search.Input{Query: "Rennsteig", Types: []string{"activity"}})
		if err != nil {
			t.Fatalf("search after %s: %v", kind, err)
		}
		hit := hitFor(page, ids.UUID(logged.Id))
		if hit == nil {
			t.Fatalf("a %s did not appear in search at all", kind)
		}
		if hit.EmailSummary != nil {
			t.Errorf("a %s hit carried an email_summary: %+v — only an email has an email's shape",
				kind, *hit.EmailSummary)
		}
		if hit.Title == "" {
			t.Errorf("a %s hit lost its generic title", kind)
		}
	}
}

// The batch enriches EVERY email on a full page, not just the first. It is
// one statement over the page's ids, and the failure this catches is a batch
// that quietly reads a prefix — which on a page of one looks exactly right.
func TestEveryEmailOnAFullPageCarriesItsRow(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	const mails = 12
	for i := range mails {
		subject := "Rennsteig thread message"
		body := "line one of message"
		if _, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
			Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
			Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
		}); err != nil {
			t.Fatalf("log %d: %v", i, err)
		}
	}

	page, err := searchEmailStore(e).Search(author, search.Input{
		Query: "Rennsteig", Types: []string{"activity"}, Limit: mails,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Hits) < mails {
		t.Fatalf("found %d of %d seeded mails", len(page.Hits), mails)
	}
	for i := range page.Hits {
		if page.Hits[i].EmailSummary == nil {
			t.Fatalf("hit %d of a page of %d carried no summary; the batch missed a row",
				i, len(page.Hits))
		}
	}
}

// The batch reader carries its OWN content gate, proven here rather than
// through search: search's activity branch is content-gated too, so a test
// that goes through search passes even when this gate is weakened. Two guards
// that both hold are one guard until each is asked separately, and this reader
// is exported — a future caller reaching it through a discover-gated list would
// otherwise print a limited conversation's subject to a colleague.
func TestTheEmailSummaryBatchRefusesAWithheldRowOnItsOwn(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	subject, body := "Severance figures", "the agreed figure is confidential"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)

	read := func(ctx context.Context) map[ids.UUID]crmcontracts.EmailSummary {
		t.Helper()
		var got map[ids.UUID]crmcontracts.EmailSummary
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var readErr error
			got, readErr = activities.EmailSummariesByIDBatch(ctx, tx, []ids.UUID{id})
			return readErr
		}); err != nil {
			t.Fatalf("batch read: %v", err)
		}
		return got
	}

	// Unlimited: the colleague reads the row, which is what makes its absence
	// below the audience's doing.
	if _, ok := read(colleague)[id]; !ok {
		t.Fatal("the colleague got no row for an unlimited mail; the fixture proves nothing")
	}

	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](id),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	if row, ok := read(colleague)[id]; ok {
		t.Errorf("the batch handed a colleague a limited mail's row: %+v", row)
	}
	if _, ok := read(author)[id]; !ok {
		t.Error("the batch withheld the author's own mail from them")
	}
}

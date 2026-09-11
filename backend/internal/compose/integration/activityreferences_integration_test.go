// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The receipts behind a relationship score, named for the reader who holds it.
//
// Two gates decide what a row says, and only a real database runs them: DISCOVER
// admits the row at all, CONTENT decides whether the subject travels. The
// distinction is the whole point — a reader may be entitled to know an exchange
// happened without being entitled to read what it said, and a projection that
// collapsed the two would either hide the row or print the words.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// namedFor runs the projection in a transaction, the way compose does.
func namedFor(
	ctx context.Context, t *testing.T, e *Env, wanted []ids.UUID,
) map[ids.UUID]crmcontracts.ActivityReference {
	t.Helper()
	var out map[ids.UUID]crmcontracts.ActivityReference
	err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var readErr error
		out, readErr = activities.ReferencesByID(ctx, tx, wanted)
		return readErr
	})
	if err != nil {
		t.Fatalf("naming the activities: %v", err)
	}
	return out
}

// A reader inside the message's audience gets the row and its words.
func TestAReceiptCarriesItsSubjectForAReaderWhoMayReadIt(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)

	subject, body := "Depot slot confirmed", "Facilities signed off this morning."
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)

	named := namedFor(author, t, e, []ids.UUID{id})

	ref, ok := named[id]
	if !ok {
		t.Fatal("the author's own activity was not named; the score's receipts are unreadable")
	}
	if ref.ContentState != crmcontracts.ActivityReferenceContentStateAvailable {
		t.Errorf("content_state = %q, want available for the author's own mail", ref.ContentState)
	}
	if ref.Subject == nil || *ref.Subject != subject {
		t.Errorf("subject = %v, want the activity's own %q", ref.Subject, subject)
	}
	if ref.Kind != crmcontracts.ActivityReferenceKindEmail {
		t.Errorf("kind = %q, want email", ref.Kind)
	}
	// The canonical email row, which the contract promises whenever the
	// activity is an email this reader may receive a summary of. Without it a
	// client cannot draw the row the way every other cited message is drawn,
	// and the receipt opens nothing.
	if ref.EmailSummary == nil {
		t.Fatal("a readable email receipt carries no email row, so it cannot be opened")
	}
	if ref.EmailSummary.ActivityId != logged.Id {
		t.Errorf("the email row names %v, want the receipt's own activity %v",
			ref.EmailSummary.ActivityId, logged.Id)
	}
}

// A receipt that is not an email carries no email row. The reader's own kind
// clause decides, and asking for one would spend the statement on a row that
// can only come back absent.
func TestANonEmailReceiptCarriesNoEmailRow(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)

	subject := "Rang about the retrofit"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "call", Subject: &subject,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)

	ref, ok := namedFor(author, t, e, []ids.UUID{id})[id]
	if !ok {
		t.Fatal("the author's own call was not named")
	}
	if ref.Kind != crmcontracts.ActivityReferenceKindCall {
		t.Errorf("kind = %q, want call", ref.Kind)
	}
	if ref.EmailSummary != nil {
		t.Error("a call carries an email row")
	}
	// It is still NAMED: the row a reader recognises is the subject, and a
	// call with no email row is not a call with no receipt.
	if ref.Subject == nil || *ref.Subject != subject {
		t.Errorf("subject = %v, want the call's own %q", ref.Subject, subject)
	}
}

// A colleague outside a limited message's audience keeps the ROW and loses the
// WORDS. That is the case the two-gate split exists for: dropping the row would
// shorten a list the count does not shorten, and printing the subject would say
// what the audience decided they may not read.
func TestAReceiptKeepsItsRowAndLosesItsWordsOutsideTheAudience(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)

	subject, body := "Severance terms", "the agreed figure is confidential"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.UUID(logged.Id)

	// The admit case first, against the SAME colleague and the same row: a
	// refusal test on its own passes against a reader who sees nothing.
	before := namedFor(colleague, t, e, []ids.UUID{id})
	if ref, ok := before[id]; !ok || ref.Subject == nil {
		t.Fatalf("the colleague could not read an unlimited mail; the refusal below proves nothing (%+v)", before[id])
	}

	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](id),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	after := namedFor(colleague, t, e, []ids.UUID{id})
	ref, ok := after[id]
	if !ok {
		t.Fatal("a limited mail vanished from the colleague's receipts; the row is discoverable, only its words are not")
	}
	if ref.ContentState != crmcontracts.ActivityReferenceContentStateWithheld {
		t.Errorf("content_state = %q, want withheld", ref.ContentState)
	}
	if ref.Subject != nil {
		t.Errorf("a limited mail's subject reached a colleague outside its audience: %q", *ref.Subject)
	}
	// The markers stay, which is what makes the row worth drawing at all.
	if ref.OccurredAt.IsZero() {
		t.Error("the withheld row carries no date, so it says nothing a reader can place")
	}
}

// A seat with no activity grant names nothing, and the score still renders.
func TestASeatWithoutTheActivityGrantNamesNoReceipts(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)

	subject := "Depot slot confirmed"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	// Every grant a rep working contacts holds EXCEPT activity, so what is
	// tested is a seat that reads contacts and not their conversations.
	ungranted := e.As(e.Rep2, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		RowScope: principal.RowScopeTeam,
	})

	named := namedFor(ungranted, t, e, []ids.UUID{ids.UUID(logged.Id)})

	if len(named) != 0 {
		t.Errorf("a seat with no activity grant named %d receipt(s)", len(named))
	}
}

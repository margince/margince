// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The email viewer's read is the one place a reader meets a message whole, so
// it is the one place every arm of the audience model has to answer the same
// way the timeline does. These tests are the matrix: each audience against
// each kind of reader, and then the state machines that are not audiences at
// all — statutory retention, a non-email row, blind recipients — kept separate
// because folding them into the matrix would multiply cases that share no rule.

// logEmailActivity logs one email against a contact and answers its id.
func logEmailActivity(author context.Context, t *testing.T, e *Env, contact ids.UUID, subject, body string) ids.ActivityID {
	t.Helper()
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

// TestEmailPresentationAccessMatrix walks every audience against every reader.
//
// The rule it holds: an audience decides who reads CONTENT, and row scope
// decides who discovers the row at all. They are separate gates, so a reader
// can be admitted by one and refused by the other — and an unbounded admin is
// admitted by scope and still refused by the audience, because an admin
// reading a colleague's limited mail is the disclosure the limit exists to
// prevent.
func TestEmailPresentationAccessMatrix(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	participant := e.As(e.Rep2, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	for _, tc := range []struct {
		audience string
		// readers that must read the content whole.
		available []struct {
			name string
			ctx  context.Context
		}
		// readers that must see the row and none of its words.
		withheld []struct {
			name string
			ctx  context.Context
		}
	}{
		{
			audience: "workspace",
			available: []struct {
				name string
				ctx  context.Context
			}{
				{"the author", author},
				{"a colleague in another team", colleague},
				{"an unbounded admin", e.Admin()},
			},
		},
		{
			audience: "participants",
			available: []struct {
				name string
				ctx  context.Context
			}{{"the author", author}},
			withheld: []struct {
				name string
				ctx  context.Context
			}{
				{"a colleague in another team", colleague},
				// The one that matters most: scope admits the admin, the
				// audience does not, and the audience wins.
				{"an unbounded admin", e.Admin()},
				{"a colleague who is not a participant", participant},
			},
		},
	} {
		t.Run(tc.audience, func(t *testing.T) {
			id := logEmailActivity(author, t, e, contact, "Q3 renewal terms", "confidential pricing")
			if _, err := e.Activities.SetAudience(author, id,
				activities.SetAudienceInput{Audience: tc.audience}); err != nil {
				t.Fatalf("limiting to %s: %v", tc.audience, err)
			}
			for _, reader := range tc.available {
				got, err := e.Activities.GetEmailPresentation(reader.ctx, id, nil)
				if err != nil {
					t.Fatalf("%s reading a %s message: %v", reader.name, tc.audience, err)
				}
				if got.Access.ContentState != crmcontracts.EmailAccessContentStateAvailable {
					t.Errorf("%s: content_state=%v, want available", reader.name, got.Access.ContentState)
				}
				if got.Summary.Subject == nil || *got.Summary.Subject != "Q3 renewal terms" {
					t.Errorf("%s: subject %v, want the subject", reader.name, got.Summary.Subject)
				}
				if got.Body == nil {
					t.Errorf("%s: no body on a message they may read", reader.name)
				}
			}
			for _, reader := range tc.withheld {
				got, err := e.Activities.GetEmailPresentation(reader.ctx, id, nil)
				if err != nil {
					t.Fatalf("%s reading a %s message: %v — a discoverable row is read, withheld", reader.name, tc.audience, err)
				}
				assertWithholdsEverything(t, reader.name, got)
			}
		})
	}
}

// assertWithholdsEverything is the leak check every withheld case runs, so
// each of them is held to the same list. A field added to the presentation
// that carries content is a field this has to learn about.
func assertWithholdsEverything(t *testing.T, who string, got crmcontracts.EmailPresentation) {
	t.Helper()
	if got.Access.ContentState != crmcontracts.EmailAccessContentStateWithheld {
		t.Errorf("%s: content_state=%v, want withheld", who, got.Access.ContentState)
	}
	if got.Summary.DisplayStatus != crmcontracts.EmailAccessStatusWithheld {
		t.Errorf("%s: display_status=%v, want withheld", who, got.Summary.DisplayStatus)
	}
	if got.Summary.Subject != nil {
		t.Errorf("%s: subject %q leaked", who, *got.Summary.Subject)
	}
	if got.Summary.Preview != nil {
		t.Errorf("%s: preview %q leaked", who, *got.Summary.Preview)
	}
	if got.Body != nil {
		t.Errorf("%s: body %q leaked", who, *got.Body)
	}
	if got.ThreadKey != nil {
		t.Errorf("%s: thread_key %q leaked", who, *got.ThreadKey)
	}
	if len(got.From) != 0 || len(got.To) != 0 || len(got.Cc) != 0 || len(got.Bcc) != 0 {
		t.Errorf("%s: participants leaked (from=%d to=%d cc=%d bcc=%d)",
			who, len(got.From), len(got.To), len(got.Cc), len(got.Bcc))
	}
	if len(got.Attachments) != 0 {
		t.Errorf("%s: %d attachment(s) leaked", who, len(got.Attachments))
	}
	if got.Thread != nil {
		t.Errorf("%s: the thread leaked", who)
	}
	if got.Access.Explanation != nil {
		// The reason describes what the message is about, so it is withheld
		// with the content.
		t.Errorf("%s: the reason %q leaked", who, *got.Access.Explanation)
	}
	if got.Access.SelectedMembers != nil && len(*got.Access.SelectedMembers) != 0 {
		t.Errorf("%s: selected-member identities leaked", who)
	}
	if got.Access.CanChange {
		t.Errorf("%s: offered a control over a message they may not read", who)
	}
	// The row itself survives: a withheld message is visibly withheld rather
	// than absent, or a reader cannot tell it from a conversation that never
	// happened.
	if got.Summary.OccurredAt.IsZero() {
		t.Errorf("%s: no occurred_at — a withheld row still says when", who)
	}
}

// TestEmailPresentationRefusesWhatIsNotAnEmail holds the kind boundary. An
// activity is not a synonym for an email: a call, a note and a task are
// activities too, and none of them has an email's shape.
func TestEmailPresentationRefusesWhatIsNotAnEmail(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	for _, kind := range []string{"call", "note", "meeting", "task"} {
		t.Run(kind, func(t *testing.T) {
			subject := "not an email"
			logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
				Kind: kind, Subject: &subject,
				Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
			})
			if err != nil {
				t.Fatalf("log a %s: %v", kind, err)
			}
			id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
			// Not-found rather than a wrong-kind error: naming the kind would
			// answer a question about a row the caller may only discover.
			if _, err := e.Activities.GetEmailPresentation(author, id, nil); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("presentation of a %s → %v, want ErrNotFound", kind, err)
			}
		})
	}
}

// TestEmailSummaryRidesEveryActivityRow holds the field a canonical row reads.
// Present exactly when kind=email, so a reader branches on the field rather
// than on the kind word.
func TestEmailSummaryRidesEveryActivityRow(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	id := logEmailActivity(author, t, e, contact, "Q3 renewal terms",
		"Können wir Dienstag sprechen?\n\nViele Grüße\nAna")
	got, err := e.Activities.GetActivity(author, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.EmailSummary == nil {
		t.Fatal("an email row carries no email_summary; every canonical row reads it")
	}
	// The preview is the sender's own words: the sign-off is not the message.
	if got.EmailSummary.Preview == nil || *got.EmailSummary.Preview != "Können wir Dienstag sprechen?" {
		t.Errorf("preview = %v, want the sentence without the sign-off", got.EmailSummary.Preview)
	}
	if got.EmailSummary.Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Errorf("move = %v on an inbound message, want needs_reply", got.EmailSummary.Move)
	}

	subject := "a call"
	call, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "call", Subject: &subject,
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log a call: %v", err)
	}
	if call.EmailSummary != nil {
		t.Error("a call carries an email_summary; only an email has an email's shape")
	}
}

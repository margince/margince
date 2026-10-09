// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The nightly display-name repair replaces only a name capture guessed. A name
// an agent, an import or a human chose survives it. So does any display that
// already names first and last name plus more.

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/auditverb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureCounterparty runs the capture ensure for one sender, as the Gmail sink
// does, and answers the contact it created or landed on.
func captureCounterparty(t *testing.T, e *integration.Env, display, email string) ids.ContactID {
	t.Helper()
	activity := seedCapturedMail(t, e, email, "hello")
	res, err := e.Contacts.EnsureCounterparty(e.ConnectorOwnerCtx(e.AdminUser, "gmail"), contacts.EnsureCounterpartyInput{
		Email: email, DisplayName: display, Domain: email[strings.IndexByte(email, '@')+1:],
		OwnerID: e.AdminUser, ActivityID: ids.From[ids.ActivityKind](activity),
		Source: "gmail:" + activity.String(), CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("capturing %q <%s>: %v", display, email, err)
	}
	return res.ContactID
}

// learnSplitName puts first_name and the surname Welter into the split columns
// and leaves full_name as it was. That is the state the old fill left behind,
// which no current writer produces any more, so it is planted directly.
func learnSplitName(t *testing.T, e *integration.Env, id ids.ContactID, first string) {
	t.Helper()
	e.WsExec(t, `UPDATE contact SET first_name = $2, last_name = 'Welter' WHERE id = $1`, id, first)
}

// runDisplayNameRepair drains the repair under the principal the nightly
// participant_backfill job binds.
func runDisplayNameRepair(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: contacts.DisplayNameRepairActor,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	w := &participantBackfillWorker{pool: e.Pool, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if _, err := w.refreshDisplayNamesWorkspace(ctx); err != nil {
		t.Fatalf("running the display-name repair: %v", err)
	}
}

func shownName(t *testing.T, e *integration.Env, id ids.ContactID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT full_name FROM contact WHERE id = $1`, id)
}

func TestTheRepairShowsTheLearnedNameInPlaceOfCapturesInitials(t *testing.T) {
	e := integration.Setup(t)
	id := captureCounterparty(t, e, "Bw", "bw@welter.test")
	learnSplitName(t, e, id, "Björn")

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != "Björn Welter" {
		t.Errorf("full_name = %q, want the learned name in place of the label capture read off the invitation", got)
	}
}

// An agent chose both names below. The first carries a nickname beside the
// pair; the second shares nothing with the pair, so only its origin keeps it.
func TestTheRepairKeepsADisplayNameAnAgentChose(t *testing.T) {
	e := integration.Setup(t)
	agent := e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms)
	first, last := "Robert", "Fischer"
	for _, in := range []contacts.CreateContactInput{
		{
			FullName: "Contact A (Nick)", Source: "manual",
			Emails: []contacts.ContactEmailInput{{Email: "a@example.test", EmailType: "work", IsPrimary: true}},
		},
		{FullName: "Bobby", FirstName: &first, LastName: &last, Source: "manual"},
	} {
		created, err := e.Contacts.CreateContact(agent, in)
		if err != nil {
			t.Fatalf("creating %q as an agent: %v", in.FullName, err)
		}
		id := ids.From[ids.ContactKind](ids.UUID(created.Id))
		if in.FirstName == nil {
			// Mail from the address teaches the record its split name, which
			// is how the agent's contact reached the repair in the first place.
			captureCounterparty(t, e, "Contact A", "a@example.test")
			if got := e.WsScalar(t, `SELECT coalesce(first_name, '') FROM contact WHERE id = $1`, id); got != "Contact" {
				t.Fatalf("first_name = %q after the mail, want the fill to have learned it", got)
			}
		}

		runDisplayNameRepair(t, e)

		if got := shownName(t, e, id); got != in.FullName {
			t.Errorf("full_name = %q, want %q: an agent chose it, and neither the fill nor the repair may replace it",
				got, in.FullName)
		}
	}
}

// A vCard ingest runs as the mailbox's connector, as capture does, and still
// copies a name somebody curated in their address book.
func TestTheRepairKeepsADisplayNameAVCardImportChose(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.ConnectorOwnerCtx(e.AdminUser, "gmail")
	results, err := e.Contacts.ImportVCardsFromMessage(ctx, []contacts.VCardEntry{{
		FullName: "Bobby (Acme board)",
		Emails:   []contacts.VCardChannel{{Value: "bob@acme.test", Kind: "work"}},
	}}, seedCapturedMail(t, e, "bob@acme.test", "my card"))
	if err != nil || len(results) != 1 || results[0].ContactID == nil {
		t.Fatalf("importing the card: results=%+v err=%v", results, err)
	}
	id := *results[0].ContactID
	captureCounterparty(t, e, "Robert Fischer", "bob@acme.test")

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != "Bobby (Acme board)" {
		t.Errorf("full_name = %q, want the name the card carried", got)
	}
}

// Capture's own guess survives when it already names both halves and says
// more. Today's parser strips an affiliation before it stores the name, so the
// display is planted the way an older capture kept it.
func TestTheRepairKeepsACapturedNameThatSaysMoreThanThePair(t *testing.T) {
	e := integration.Setup(t)
	id := captureCounterparty(t, e, "Björn Welter", "bjoern@gradion.test")
	e.WsExec(t, `UPDATE contact SET full_name = 'Welter, Björn (Gradion)' WHERE id = $1`, id)

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != "Welter, Björn (Gradion)" {
		t.Errorf("full_name = %q, want the display that already names both halves", got)
	}
}

func TestTheRepairKeepsADisplayNameAHumanEdited(t *testing.T) {
	e := integration.Setup(t)
	id := captureCounterparty(t, e, "Bw", "bw@welter.test")
	typed := "BW"
	if _, err := e.Contacts.UpdateContact(e.Admin(), id, contacts.UpdateContactInput{FullName: &typed}); err != nil {
		t.Fatalf("a human renaming the contact: %v", err)
	}
	learnSplitName(t, e, id, "Björn")

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != typed {
		t.Errorf("full_name = %q, want %q: a human typed it", got, typed)
	}
}

// An agent that renames a captured contact chose that name, although the row
// still carries capture's captured_by.
func TestTheRepairKeepsADisplayNameAnAgentEdited(t *testing.T) {
	e := integration.Setup(t)
	id := captureCounterparty(t, e, "Bw", "bw@welter.test")
	chosen := "Bobby (VIP)"
	agent := e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms)
	if _, err := e.Contacts.UpdateContact(agent, id, contacts.UpdateContactInput{FullName: &chosen}); err != nil {
		t.Fatalf("an agent renaming the contact: %v", err)
	}
	learnSplitName(t, e, id, "Björn")

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != chosen {
		t.Errorf("full_name = %q, want %q: an agent chose it", got, chosen)
	}
}

// A restore writes full_name under its own audit verb, and is as much a choice
// as an update.
func TestTheRepairKeepsADisplayNameAHumanRestored(t *testing.T) {
	e := integration.Setup(t)
	id := captureCounterparty(t, e, "Bw", "bw@welter.test")
	restored := "Bee Dub"
	if _, err := e.Contacts.UpdateContact(e.Admin(), id, contacts.UpdateContactInput{
		FullName: &restored,
		Trail:    auditverb.Trail{Verb: auditverb.Restore},
	}); err != nil {
		t.Fatalf("a human restoring the contact's name: %v", err)
	}
	learnSplitName(t, e, id, "Björn")

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != restored {
		t.Errorf("full_name = %q, want %q: a human restored it", got, restored)
	}
}

// The fill keeps a display that already names the pair it learns and says
// more. The row is planted the way a capture stored it before the parser
// stripped affiliations and filled the split columns.
func TestTheFillKeepsACapturedNameThatSaysMoreThanThePair(t *testing.T) {
	e := integration.Setup(t)
	id := captureCounterparty(t, e, "Robert Fischer", "bob@acme.test")
	e.WsExec(t, `UPDATE contact SET full_name = 'Robert Fischer (Acme board)', first_name = NULL, last_name = NULL
		WHERE id = $1`, id)

	captureCounterparty(t, e, "Robert Fischer", "bob@acme.test")

	if got := e.WsScalar(t, `SELECT coalesce(first_name, '') FROM contact WHERE id = $1`, id); got != "Robert" {
		t.Fatalf("first_name = %q, want the fill to have learned it", got)
	}
	if got := shownName(t, e, id); got != "Robert Fischer (Acme board)" {
		t.Errorf("full_name = %q, want the display that already names both halves", got)
	}
}

// A blank half is no name. The repair neither reads it as contained in the
// display nor writes a one-part name in its place.
func TestTheRepairLeavesAContactWithABlankHalfAlone(t *testing.T) {
	e := integration.Setup(t)
	id := captureCounterparty(t, e, "Bw", "bw@welter.test")
	learnSplitName(t, e, id, " ")

	runDisplayNameRepair(t, e)

	if got := shownName(t, e, id); got != "Bw" {
		t.Errorf("full_name = %q, want the label kept until both halves are known", got)
	}
}

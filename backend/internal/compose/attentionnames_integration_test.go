// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// attentionNames against real Postgres and the real row scope: a label is
// exactly as visible as the record behind it. A person this reader may see
// answers their name; a capture-private person owned by somebody else
// answers absent — never an error, and never the name — because the label
// pass must degrade a card, not disclose a record or empty a lane.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func namesOver(e *integration.Env) attentionNames {
	db := InstallationDB(e.Pool)
	return attentionNames{
		people:     people.NewStore(db),
		deals:      deals.NewStore(db, DealsInstallation()),
		activities: activities.NewStore(db),
		projects:   projects.NewStore(db),
	}
}

func TestALabelIsExactlyAsVisibleAsItsRecord(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)

	visible := e.SeedPerson(t, "Dana Weiss", nil)
	private := e.SeedPerson(t, "Zeta Privatkontakt", &e.Rep3)
	e.MakeCapturePrivate(t, "person", private, e.Rep3)

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)

	// BOTH in one ask, which is the batched seam's whole shape and also the
	// sharper test: the visible one must come back named and the private one
	// must not, from the same query. A batch that answered per-set rather
	// than per-row would either name both or name neither.
	labels, err := names.Labels(reader, "person", []ids.UUID{visible, private})
	if err != nil {
		t.Fatalf("naming a page of people: %v", err)
	}
	if labels[visible] != "Dana Weiss" {
		t.Fatalf("label = %q, want the person's name — a resolver that cannot name a readable record makes every card anonymous", labels[visible])
	}
	if label, named := labels[private]; named {
		t.Fatalf("label = %q for another rep's capture-private contact — the label pass just disclosed a record the row scope hides", label)
	}
}

// A record with no name of its own is ABSENT rather than blank: a card
// showing an empty string where a record should be says less than one
// showing nothing, and the single-record form answered ok=false for it.
func TestARecordWithNoNameIsAbsentRatherThanBlank(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	nameless := e.SeedPerson(t, "", nil)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)

	labels, err := names.Labels(reader, "person", []ids.UUID{nameless})
	if err != nil {
		t.Fatalf("naming a nameless person: %v", err)
	}
	if label, named := labels[nameless]; named {
		t.Fatalf("label = %q for a person captured without a name, want absent", label)
	}
}

// An id nobody has ever minted is absent, not an error: the producer's claim
// that a reference exists is not this resolver's to fail the page over.
func TestAnIdThatNamesNothingIsAbsentNotAnError(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)

	labels, err := names.Labels(reader, "person", []ids.UUID{ids.NewV7()})
	if err != nil {
		t.Fatalf("naming a record that does not exist: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("labels = %v for an id naming nothing, want none", labels)
	}
}

func TestAnUnknownSubjectTypeAnswersAbsentNotAGuess(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	labels, err := names.Labels(reader, "warehouse", []ids.UUID{ids.NewV7()})
	if err != nil || len(labels) != 0 {
		t.Fatalf("labels=%v err=%v for a type outside the vocabulary, want none and no error", labels, err)
	}
}

// The context matters: the same record, asked for without a principal,
// must refuse into absence rather than leak through the resolver.
func TestTheResolverHoldsNoAuthorityOfItsOwn(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	person := e.SeedPerson(t, "Dana Weiss", nil)
	labels, err := names.Labels(context.Background(), "person", []ids.UUID{person})
	if err == nil && len(labels) > 0 {
		t.Fatal("an unauthenticated ask was answered — the resolver carries authority its callers never granted")
	}
}

// A subject line is CONTENT, and the batch must withhold it exactly as the
// single-record read does.
//
// The card names the message a rep is being told about; on a limited thread
// the reader may know the row exists and may not read what it says. A batched
// read that filtered on the audience instead of selecting THROUGH it would
// have answered the row's subject to everyone who could discover it.
func TestALimitedMessagesSubjectIsWithheldFromTheBatchToo(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	owner := integration.OwnerConn(t)

	open := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Renewal terms', now(), 'manual', 'human:x')`)
	limited := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Board compensation', now(), 'manual', 'human:someone-else')`)
	if _, err := owner.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, limited); err != nil {
		t.Fatalf("limiting the message: %v", err)
	}

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	labels, err := names.Labels(reader, "activity", []ids.UUID{open, limited})
	if err != nil {
		t.Fatalf("naming a page of messages: %v", err)
	}
	if labels[open] != "Renewal terms" {
		t.Errorf("label = %q for an open message, want its subject", labels[open])
	}
	if label, named := labels[limited]; named {
		t.Fatalf("label = %q for a message this reader is not on — the batch just disclosed what a limited thread says", label)
	}
}

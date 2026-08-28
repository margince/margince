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

	label, ok, err := names.Label(reader, "person", visible)
	if err != nil {
		t.Fatalf("naming a visible person: %v", err)
	}
	if !ok || label != "Dana Weiss" {
		t.Fatalf("label = %q ok=%v, want the person's name — a resolver that cannot name a readable record makes every card anonymous", label, ok)
	}

	label, ok, err = names.Label(reader, "person", private)
	if err != nil {
		t.Fatalf("a withheld record must cost the label, not fail the read: %v", err)
	}
	if ok || label != "" {
		t.Fatalf("label = %q ok=%v for another rep's capture-private contact — the label pass just disclosed a record the row scope hides", label, ok)
	}
}

func TestAnUnknownSubjectTypeAnswersAbsentNotAGuess(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	if _, ok, err := names.Label(reader, "warehouse", ids.NewV7()); err != nil || ok {
		t.Fatalf("ok=%v err=%v for a type outside the vocabulary, want absent and no error", ok, err)
	}
}

// The context matters: the same record, asked for without a principal,
// must refuse into absence rather than leak through the resolver.
func TestTheResolverHoldsNoAuthorityOfItsOwn(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	person := e.SeedPerson(t, "Dana Weiss", nil)
	if _, ok, err := names.Label(context.Background(), "person", person); err == nil && ok {
		t.Fatal("an unauthenticated ask was answered — the resolver carries authority its callers never granted")
	}
}

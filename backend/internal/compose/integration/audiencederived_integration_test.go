// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a limited message produces for readers outside its audience, and what
// happens to that produce when a message is limited after the fact.
//
// The audience column is only worth what the readers make of it. These are the
// four system readers that used to ignore it — the embedding generator, the
// signature miner, the attention-label backlog and the subject-access export —
// plus the retraction that has to follow a narrowing, because narrowing a row
// changes who may read the ROW and reaches nothing that was derived from it.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldMailSubject and heldMailBody are the fixture's confidential message. They
// are distinct strings so an assertion can say WHICH half of the indexed text
// escaped.
const (
	heldMailSubject = "Aufhebungsvertrag Entwurf"
	heldMailBody    = "anbei der Entwurf, bitte streng vertraulich behandeln"
)

func TestAHeldActivityIsNeverEmbeddedAndLosesItsVectorWhenNarrowed(t *testing.T) {
	e := SetupSearch(t)
	fake := ai.NewFakeClient()
	gen := search.NewEmbedGen(e.Store, fakeEmbedder(t, fake))
	ctx := context.Background()

	held := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', $2, $3, now(), 'inbound', 'gmail', 'connector:gmail:x', 'participants')`,
		heldMailSubject, heldMailBody)
	open := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', $2, $3, now(), 'inbound', 'gmail', 'connector:gmail:x', 'workspace')`,
		heldMailSubject, heldMailBody)

	captured := func(id ids.UUID) kevents.Envelope {
		return kevents.Envelope{
			EventID: ids.NewV7(),
			Type:    "activity.captured",
			Entity:  kevents.EntityRef{Type: "activity", ID: id},
		}
	}
	for _, id := range []ids.UUID{held, open} {
		if err := gen.HandleEvent(ctx, captured(id)); err != nil {
			t.Fatal(err)
		}
	}

	vectors := func(id ids.UUID) int {
		t.Helper()
		var n int
		if err := e.Owner.QueryRow(ctx, `
			SELECT count(*) FROM embedding WHERE entity_type = 'activity' AND entity_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// The open message proves the fixture indexes at all. Without it a broken
	// embedder would produce the same "no vector" answer as a working gate.
	if vectors(open) != 1 {
		t.Fatal("the workspace message was not embedded — the fixture cannot tell a working gate from a broken embedder")
	}
	if vectors(held) != 0 {
		t.Error("a message limited to its participants was embedded — its text is now retrievable by semantic neighbourhood for every seat")
	}

	// Narrowing the open one has to take the vector with it. The embedding
	// generator cannot do this itself: its query returns no row for a narrowed
	// message, and no row means "nothing to index", not "delete what is there".
	if _, err := e.Owner.Exec(ctx, `UPDATE activity SET audience = 'participants' WHERE id = $1`, open); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"changed_fields": map[string]any{"audience": "participants"}})
	if err != nil {
		t.Fatal(err)
	}
	rescope := compose.NewAudienceRescopeGen(e.Pool)
	if err := rescope.HandleEvent(ctx, kevents.Envelope{
		Type:    "activity.updated",
		Entity:  kevents.EntityRef{Type: "activity", ID: open},
		Payload: payload,
	}); err != nil {
		t.Fatalf("the audience-rescope consumer refused the narrowing: %v", err)
	}
	if vectors(open) != 0 {
		t.Error("narrowing the message left its vector behind — a similarity probe still reaches the text the limit was meant to withhold")
	}

	// And widening re-indexes, so the retraction is not a one-way door.
	if _, err := e.Owner.Exec(ctx, `UPDATE activity SET audience = 'workspace' WHERE id = $1`, open); err != nil {
		t.Fatal(err)
	}
	if err := gen.HandleEvent(ctx, captured(open)); err != nil {
		t.Fatal(err)
	}
	if vectors(open) != 1 {
		t.Error("a re-opened message was not re-embedded — narrowing it once removed it from search for good")
	}
}

// The subject-access export is the third system reader, and it is the one with
// a genuine conflict: Art. 15 owes the subject what is held about them, and a
// limited message is held about them AND is a colleague's private
// correspondence. It resolves by disclosing the fact and withholding the text,
// which is also what makes a later release meaningful — the subject can see
// there is something to ask about.
func TestASubjectAccessExportListsAHeldActivityWithoutItsText(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	ctx := context.Background()
	person := e.SeedPerson(t, "Held Correspondent", &e.Rep1)

	seed := func(subject, audience string) ids.UUID {
		t.Helper()
		id := ids.NewV7()
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
			VALUES ($1, 'email', $2, $3, now(), 'inbound', 'gmail', 'connector:gmail:`+e.Rep1.String()+`', $4)`,
			id, subject, heldMailBody, audience); err != nil {
			t.Fatal(err)
		}
		LinkActivity(t, owner, id, "person", person)
		return id
	}
	open := seed("ordinary order confirmation", "workspace")
	held := seed(heldMailSubject, "participants")

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.PersonKind](person))
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	rows := map[ids.UUID]map[string]any{}
	for _, row := range pkg.Activities {
		id, ok := row["id"].(ids.UUID)
		if !ok {
			t.Fatalf("an exported activity carries no id: %#v", row)
		}
		rows[id] = row
	}
	if len(rows) != 2 {
		t.Fatalf("the export carried %d activities, want both — a limited message is still HELD about the subject and Art. 15 owes its existence", len(rows))
	}
	// The open one proves the export discloses at all.
	if got := rows[open]["subject"]; got != "ordinary order confirmation" {
		t.Errorf("the open message's subject came back as %#v — the fixture cannot tell a working gate from a broken export", got)
	}
	if got := rows[held]["subject"]; got != nil {
		t.Errorf("the limited message's subject was exported as %#v — a colleague's private correspondence in a package the subject keeps a copy of", got)
	}
	if got := rows[held]["body"]; got != nil {
		t.Errorf("the limited message's body was exported as %#v", got)
	}
	if got := rows[held]["content_disclosed"]; got != false {
		t.Errorf("content_disclosed for the limited message = %#v, want false — the package must say the text was withheld rather than look like there was none", got)
	}
	if rows[held]["withheld_from_mailbox_of"] == nil {
		t.Error("the withheld row names no mailbox — the operator has nobody to ask for a release")
	}
}

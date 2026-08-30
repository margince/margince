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

	// The package holds raw row maps, so the id arrives as the driver's own
	// [16]byte rather than as ids.UUID. Keying on the string form is what lets
	// the assertions name a specific message.
	rows := map[string]map[string]any{}
	for _, row := range pkg.Activities {
		raw, ok := row["id"].([16]byte)
		if !ok {
			t.Fatalf("an exported activity carries no id: %#v", row)
		}
		rows[ids.UUID(raw).String()] = row
	}
	if len(rows) != 2 {
		t.Fatalf("the export carried %d activities, want both — a limited message is still HELD about the subject and Art. 15 owes its existence", len(rows))
	}
	// The open one proves the export discloses at all.
	if got := rows[open.String()]["subject"]; got != "ordinary order confirmation" {
		t.Errorf("the open message's subject came back as %#v — the fixture cannot tell a working gate from a broken export", got)
	}
	if got := rows[held.String()]["subject"]; got != nil {
		t.Errorf("the limited message's subject was exported as %#v — a colleague's private correspondence in a package the subject keeps a copy of", got)
	}
	if got := rows[held.String()]["body"]; got != nil {
		t.Errorf("the limited message's body was exported as %#v", got)
	}
	if got := rows[held.String()]["content_disclosed"]; got != false {
		t.Errorf("content_disclosed for the limited message = %#v, want false — the package must say the text was withheld rather than look like there was none", got)
	}
	if rows[held.String()]["withheld_from_mailbox_of"] == nil {
		t.Error("the withheld row names no mailbox — the operator has nobody to ask for a release")
	}
}

// A stored provider original whose activity does not vouch for it stays
// withheld. The raw payload is the whole RFC822 message, matched into the
// package by a substring of the subject's address anywhere inside it — so a
// subject merely quoted in a colleague's thread pulls the thread in.
//
// The row can outlive its activity: the sink stores the original before the
// activity exists, an erasure can destroy the activity and leave the original
// under its own retention rule, and a (source_system, source_id) pair need not
// join at all. A test written as "is there a LIMITED activity for this
// original" reads every one of those as permission. This asserts the other
// direction: no open activity vouching for it, no payload.
func TestASubjectAccessExportWithholdsARawOriginalNothingVouchesFor(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	ctx := context.Background()
	person := e.SeedPerson(t, "Quoted Subject", &e.Rep1)

	var email string
	if err := owner.QueryRow(ctx, `
		INSERT INTO person_email (id, person_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, $3, 'work', true, 'manual', 'human:x') RETURNING email`,
		ids.NewV7(), person, "quoted-"+ids.NewV7().String()+"@example.test").Scan(&email); err != nil {
		t.Fatal(err)
	}

	// Three originals, one per case, each carrying the subject's address so all
	// three are pulled into the package by the same substring match.
	seedRaw := func(sourceID string) {
		t.Helper()
		if _, err := owner.Exec(ctx, `
			INSERT INTO raw_capture (id, source_system, source_id, payload, received_at)
			VALUES ($1, 'gmail', $2, jsonb_build_object('raw', 'To: '||$3::text||' — the whole message'), now())`,
			ids.NewV7(), sourceID, email); err != nil {
			t.Fatal(err)
		}
	}
	seedActivity := func(sourceID, audience string) {
		t.Helper()
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, source_system, source_id, captured_by, audience)
			VALUES ($1, 'email', $2, $3, now(), 'inbound', 'gmail', 'gmail', $4, 'connector:gmail:`+e.Rep1.String()+`', $5)`,
			ids.NewV7(), heldMailSubject, heldMailBody, sourceID, audience); err != nil {
			t.Fatal(err)
		}
	}
	// raw_capture carries UNIQUE (source_system, source_id), so the keys are
	// per-run: a fixed one collides on the insert against a database this suite
	// did not rebuild, and the test would then fail at a duplicate key rather
	// than at an assertion about the audience.
	run := ids.NewV7().String()
	open, limited, orphan := "msg-open-"+run, "msg-limited-"+run, "msg-orphan-"+run
	seedRaw(open)
	seedActivity(open, "workspace")
	seedRaw(limited)
	seedActivity(limited, "participants")
	// The orphan: an original with no activity at all.
	seedRaw(orphan)

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.PersonKind](person))
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	payloads := map[string]any{}
	for _, row := range pkg.RawCapture {
		id, ok := row["source_id"].(string)
		if !ok {
			t.Fatalf("a raw capture row carries no source_id: %#v", row)
		}
		payloads[id] = row["payload"]
	}
	if len(payloads) != 3 {
		t.Fatalf("the package listed %d originals, want all 3 — Art. 15 owes the FACT that an original is held even when its content is withheld", len(payloads))
	}
	if payloads[open] == nil {
		t.Error("the open message's original was withheld — the fixture cannot tell a working gate from a broken query")
	}
	if payloads[limited] != nil {
		t.Error("a limited message's provider original was exported in full")
	}
	if payloads[orphan] != nil {
		t.Error("an original with no activity to vouch for it was exported in full — absence of a limiting row is not permission")
	}
}

// The two write-time re-checks, which is what makes the audience hold against a
// narrowing that lands mid-pass rather than only against one that landed first.
//
// Both writers select their subject in one transaction, spend a model call, and
// write in another. A clause in the SELECT alone leaves a window exactly one
// model call wide in which a human or a verdict limits the message and the
// write lands anyway — and nothing clears it afterwards, because the narrowing
// already ran. Both re-test the row as it is at write time.
func TestAWriteThatRacesANarrowingLosesToIt(t *testing.T) {
	e := SetupSearch(t)
	fake := ai.NewFakeClient()
	gen := search.NewEmbedGen(e.Store, fakeEmbedder(t, fake))
	ctx := context.Background()

	// The generator is handed an event for a message that was open when the
	// event was written and is limited by the time the handler runs — the
	// ordinary shape of an at-least-once bus under a concurrent narrowing.
	raced := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', $2, $3, now(), 'inbound', 'gmail', 'connector:gmail:x', 'participants')`,
		heldMailSubject, heldMailBody)
	if err := gen.HandleEvent(ctx, kevents.Envelope{
		EventID: ids.NewV7(),
		Type:    "activity.captured",
		Entity:  kevents.EntityRef{Type: "activity", ID: raced},
	}); err != nil {
		t.Fatalf("the embedding generator refused the event: %v", err)
	}

	// The write-time arm is the one under test, so the text is also written
	// directly through the store — the path the generator's own SELECT would
	// have filtered, standing in for a worker that read the body before the
	// narrowing committed.
	fresh, err := e.Store.UpsertEmbedding(e.Admin(), "activity", raced,
		heldMailSubject+" "+heldMailBody, fakeEmbedder(t, fake))
	if err != nil {
		t.Fatalf("upserting the embedding: %v", err)
	}
	if fresh {
		t.Error("a vector was written for a limited message — the write-time re-check did not fire, so a worker that read the body before the narrowing reinserts it after")
	}
	var vectors int
	if err := e.Owner.QueryRow(ctx, `
		SELECT count(*) FROM embedding WHERE entity_type = 'activity' AND entity_id = $1`, raced).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	if vectors != 0 {
		t.Error("the limited message has a vector after all — a similarity probe reaches text its audience withholds")
	}

	// The same write, on an OPEN message, must still land: a re-check that
	// refused everything would pass every assertion above and break search.
	open := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', $2, $3, now(), 'inbound', 'gmail', 'connector:gmail:x', 'workspace')`,
		heldMailSubject, heldMailBody)
	if wrote, err := e.Store.UpsertEmbedding(e.Admin(), "activity", open,
		heldMailSubject+" "+heldMailBody, fakeEmbedder(t, fake)); err != nil || !wrote {
		t.Errorf("an open message was not embedded (wrote=%v err=%v) — the re-check refuses more than the audience", wrote, err)
	}
}

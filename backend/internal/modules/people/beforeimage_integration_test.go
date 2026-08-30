// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// What the module's guarded fills record about the state they replaced.
//
// Each writer here fills only a field nobody has answered — the capture name
// completion behind IS NULL on both split columns, the search and site-read
// passes behind ON CONFLICT DO NOTHING, the SLA mark behind its at-most-once
// CAS. That makes each one's before-image exact rather than approximate, and it
// is the reason these writers may name real fields at all: a writer that can
// only fill an empty field can never hold a key a human typed, so recording
// real keys cannot take field ownership away from a person.
//
// "Empty by construction" is an argument, and an argument is not what
// audit_log.before is for. These pin the row saying it.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditImagesHolding reads the images of the newest update row for one entity
// whose after-image mentions key — the write under test, rather than whichever
// row happened to land last in the same transaction.
func auditImagesHolding(ctx context.Context, t *testing.T, s *Store, entityType string, id ids.UUID, key string) (before, after map[string]any) {
	t.Helper()
	var beforeJSON, afterJSON []byte
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT before, after FROM audit_log
			  WHERE entity_type = $1 AND entity_id = $2 AND action = 'update' AND after ? $3
			  ORDER BY occurred_at DESC, id DESC LIMIT 1`, entityType, id, key,
		).Scan(&beforeJSON, &afterJSON)
	}); err != nil {
		t.Fatalf("reading the %s audit row holding %q: %v", entityType, key, err)
	}
	if beforeJSON == nil {
		t.Fatalf("before is SQL NULL — the row cannot say what %q changed from", key)
	}
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatalf("before is not an object: %v", err)
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatalf("after is not an object: %v", err)
	}
	return before, after
}

// countAuditRowsHolding counts the update rows whose after-image mentions key.
func countAuditRowsHolding(ctx context.Context, t *testing.T, s *Store, entityType string, id ids.UUID, key string) int {
	t.Helper()
	var n int
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_log
			  WHERE entity_type = $1 AND entity_id = $2 AND action = 'update' AND after ? $3`,
			entityType, id, key).Scan(&n)
	}); err != nil {
		t.Fatalf("counting %s audit rows holding %q: %v", entityType, key, err)
	}
	return n
}

// wantImage fails unless image[key] is exactly want — an absent key and a
// recorded null are different answers, so presence is asserted apart from value.
//
//craft:ignore naked-any an audit image is jsonb: every column value decodes to any, which is the type under test
func wantImage(t *testing.T, image map[string]any, label, key string, want any) {
	t.Helper()
	value, present := image[key]
	if !present {
		t.Errorf("%s has no %q key: %v", label, key, image)
		return
	}
	if value != want {
		t.Errorf("%s[%s] = %v, want %v", label, key, value, want)
	}
}

// A second mail completes a record the first left half-named, and the row says
// what each column held: nothing for the split pair, and for full_name the one
// short spelling the parser had refused to split.
func TestACompletedNameRecordsWhatEachColumnHeld(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// A lone surname names the person and says nothing about a given name, so
	// the split columns stay NULL and full_name holds only "Hoffmann".
	first, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "hoffmann@later.test", "", "later.test"))
	if err != nil || !first.PersonCreated {
		t.Fatalf("first ensure = %+v (err %v), want a created person", first, err)
	}

	second, err := e.store.EnsureCounterparty(ctx,
		e.ensureInput(ctx, t, "hoffmann@later.test", "Hanna Hoffmann", "later.test"))
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if !second.NameFilled || second.PersonID != first.PersonID {
		t.Fatalf("second ensure = %+v, want the incumbent %s completed", second, first.PersonID)
	}

	before, after := auditImagesHolding(ctx, t, e.store, entityPerson, first.PersonID.UUID, "first_name")
	wantImage(t, before, "before", "first_name", nil)
	wantImage(t, before, "before", "last_name", nil)
	wantImage(t, before, "before", "full_name", "Hoffmann")
	wantImage(t, after, "after", "first_name", "Hanna")
	wantImage(t, after, "after", "last_name", "Hoffmann")
	wantImage(t, after, "after", "full_name", "Hanna Hoffmann")
}

// The precedence guarantee for that writer. A human types the split name; a
// later confident header declines to rewrite it; and because it declined, the
// columns never reach an audit image at all — so the human stays their latest
// writer and an agent's patch of them still stages for approval.
func TestACompletedNameNeverRecordsAColumnAHumanTyped(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "wrong@human.test", "", "human.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE person SET full_name = $2, first_name = $3, last_name = $4 WHERE id = $1`,
			res.PersonID, "Wolfgang Schmitt-Rink", "Wolfgang", "Schmitt-Rink")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	again, err := e.store.EnsureCounterparty(ctx,
		e.ensureInput(ctx, t, "wrong@human.test", "Wolf Rink", "human.test"))
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if again.NameFilled {
		t.Fatalf("the second ensure reported filling a name a human had typed")
	}
	if n := countAuditRowsHolding(ctx, t, e.store, entityPerson, res.PersonID.UUID, "first_name"); n != 0 {
		t.Errorf("%d audit rows claim first_name, want none — the fill never ran", n)
	}
}

// A field a public search result answered had no prior value, and the row says
// so per field rather than leaving a reader to infer it from the statement.
func TestASearchDiscoveredFillRecordsTheEmptyFields(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	const profile = "https://linkedin.test/in/mira-halvorsen"
	applied, err := e.store.ApplyDiscoveredFields(ctx, personID, []DiscoveredField{{
		Field: "linkedin", Value: profile,
		EvidenceSnippet: "Mira Halvorsen — Head of Platform at Voltaq Systems",
		SourceRef:       "search:voltaq-mira",
	}})
	if err != nil {
		t.Fatalf("ApplyDiscoveredFields: %v", err)
	}
	if len(applied) != 1 || applied[0] != "linkedin" {
		t.Fatalf("applied = %v, want the linkedin field", applied)
	}

	before, after := auditImagesHolding(ctx, t, e.store, entityPerson, personID.UUID, "linkedin")
	wantImage(t, before, "before", "linkedin", nil)
	wantImage(t, after, "after", "linkedin", profile)
	// The source is context about the write, not a field of the contact.
	if _, folded := after[auditKeySource]; folded {
		t.Errorf("the after image carries the operation's source: %v", after)
	}
}

// The site-read fill records what the page filled and what each field held —
// nothing — and keeps the page that said so out of the images.
func TestASitePersonFillRecordsTheEmptyFields(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, orgID := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	matched, err := e.store.ApplySitePersonFields(ctx, orgID, SitePersonFields{
		Name: "Mira Halvorsen", Role: "Head of Platform",
		PublishedEmail:  "mira@voltaq.test",
		EvidenceSnippet: "Mira Halvorsen, Head of Platform",
		SourceURL:       "https://voltaq.test/team",
	})
	if err != nil {
		t.Fatalf("ApplySitePersonFields: %v", err)
	}
	if !matched {
		t.Fatal("the page's own employee was not matched — nothing was written to audit")
	}

	before, after := auditImagesHolding(ctx, t, e.store, entityPerson, personID.UUID, "role")
	wantImage(t, before, "before", "role", nil)
	wantImage(t, before, "before", "title", nil)
	wantImage(t, after, "after", "role", "Head of Platform")
	wantImage(t, after, "after", "title", "Head of Platform")
	if _, folded := after[auditKeyFields]; folded {
		t.Errorf("the after image carries a bookkeeping key instead of the fields: %v", after)
	}
}

// The signature pass fills only a column nobody has answered, and the row now
// says what that column held. The mail it read is context about the write and
// stays out of the images.
func TestASignatureFillRecordsTheEmptyColumn(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	res, err := e.store.ApplySignatureFields(ctx, personID, e.openSignatureSource(ctx, t), []SignatureField{{
		Name: "title", Value: "Head of Platform",
		Evidence: "Mira Halvorsen | Head of Platform | Voltaq", Confidence: 0.9,
	}})
	if err != nil {
		t.Fatalf("ApplySignatureFields: %v", err)
	}
	if res.Applied != 1 {
		t.Fatalf("result = %+v, want the title applied", res)
	}

	before, after := auditImagesHolding(ctx, t, e.store, entityPerson, personID.UUID, "title")
	wantImage(t, before, "before", "title", nil)
	wantImage(t, after, "after", "title", signatureFieldFilled)
	if _, folded := after[auditKeyFields]; folded {
		t.Errorf("the after image carries a bookkeeping key instead of the column: %v", after)
	}
}

// An answer that is already on the record stands, and the fill that declined
// records nothing about the field — the same precedence property, held on the
// site-read pass. The prior answer is written by the real signature writer,
// because a row this test inserted itself would prove nothing about the guard
// the two writers actually share.
//
// That writer records the field it filled, so the field's history is asserted by
// WHOSE row stands rather than by there being none: one row, and the signature's.
func TestASitePersonFillNeverRecordsAFieldAlreadyAnswered(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, orgID := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	if _, err := e.store.ApplySignatureFields(ctx, personID, e.openSignatureSource(ctx, t), []SignatureField{{
		Name: "role", Value: "Chief Platform Officer",
		Evidence: "Mira Halvorsen | Chief Platform Officer", Confidence: 0.9,
	}}); err != nil {
		t.Fatalf("seeding the earlier answer: %v", err)
	}

	if _, err := e.store.ApplySitePersonFields(ctx, orgID, SitePersonFields{
		Name: "Mira Halvorsen", Role: "Head of Platform",
		PublishedEmail:  "mira@voltaq.test",
		EvidenceSnippet: "Mira Halvorsen, Head of Platform",
		SourceURL:       "https://voltaq.test/team",
	}); err != nil {
		t.Fatalf("ApplySitePersonFields: %v", err)
	}

	if n := countAuditRowsHolding(ctx, t, e.store, entityPerson, personID.UUID, "role"); n != 1 {
		t.Fatalf("%d audit rows claim role, want only the signature's — the site fill declined", n)
	}
	_, after := auditImagesHolding(ctx, t, e.store, entityPerson, personID.UUID, "role")
	wantImage(t, after, "after", "role", signatureFieldFilled)
}

// The breach mark is the lead's own column and carries an image; the deadline
// it missed is context about the breach and rides evidence, because a lead has
// no `deadline` field for field history to project one onto.
func TestASLABreachSeparatesTheColumnFromTheDeadline(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	now := time.Now().UTC()
	overdue := e.seedLeadCreatedAt(t, "overdue@images.test", now.Add(-DefaultFirstResponseTarget-time.Hour))

	breaches, err := e.store.ScanLeadSLA(e.ctx, now)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(breaches) != 1 || breaches[0].LeadID != overdue {
		t.Fatalf("breaches = %+v, want exactly the overdue lead %s", breaches, overdue)
	}

	before, after := auditImagesHolding(e.ctx, t, e.store, "lead", overdue.UUID, slaBreachedColumn)
	wantImage(t, before, "before", slaBreachedColumn, nil)
	if after[slaBreachedColumn] == nil {
		t.Errorf("after[%s] = nil, want the instant the mark was set", slaBreachedColumn)
	}
	if _, folded := after["deadline"]; folded {
		t.Errorf("the after image carries a deadline the lead has no column for: %v", after)
	}
	if _, folded := before["deadline"]; folded {
		t.Errorf("the before image carries a deadline the lead has no column for: %v", before)
	}

	var evidence map[string]any
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(e.ctx,
			`SELECT evidence FROM audit_log
			  WHERE entity_type = 'lead' AND entity_id = $1 AND action = 'update' AND after ? $2
			  ORDER BY occurred_at DESC, id DESC LIMIT 1`, overdue, slaBreachedColumn).Scan(&raw); err != nil {
			return err
		}
		return json.Unmarshal(raw, &evidence)
	}); err != nil {
		t.Fatalf("reading the breach evidence: %v", err)
	}
	if evidence["deadline"] == nil {
		t.Errorf("evidence = %v, want the deadline the breach missed", evidence)
	}
}

// A signature field that lands only in the sidecar is recorded like the same
// field filled from a site read: an explicit null before, the value after. One
// field with two histories — a change from one writer and a blank row from
// another — is what a reader of the field's history cannot reconcile.
func TestASignatureFillRecordsTheEmptySidecarFields(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@voltaq.test", "Voltaq Systems GmbH", "voltaq.test")

	const profile = "https://linkedin.test/in/mira-halvorsen"
	res, err := e.store.ApplySignatureFields(ctx, personID, e.openSignatureSource(ctx, t), []SignatureField{
		{Name: "role", Value: "Head of Platform", Evidence: "Mira Halvorsen | Head of Platform", Confidence: 0.9},
		{Name: "linkedin", Value: profile, Evidence: "linkedin.test/in/mira-halvorsen", Confidence: 0.9},
	})
	if err != nil {
		t.Fatalf("ApplySignatureFields: %v", err)
	}
	if res.Applied != 2 {
		t.Fatalf("result = %+v, want both sidecar fields applied", res)
	}

	before, after := auditImagesHolding(ctx, t, e.store, entityPerson, personID.UUID, "role")
	wantImage(t, before, "before", "role", nil)
	wantImage(t, before, "before", "linkedin", nil)
	// Named, not quoted: this pass parses a message, and audit_log outlives the
	// erasure that clears what it parsed.
	wantImage(t, after, "after", "role", signatureFieldFilled)
	wantImage(t, after, "after", "linkedin", signatureFieldFilled)
}

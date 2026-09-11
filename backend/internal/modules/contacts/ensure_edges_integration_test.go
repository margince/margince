// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The ensure/enrich edges over a real Postgres: repeat mail reuses the
// exact-tier incumbent instead of minting twins, an impersonation-suspect
// display name lands quarantined, and the signature apply keeps its
// evidence-or-omit promise when a guarded fill loses its race — the
// evidence row is withdrawn, never left claiming an unapplied value.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ensureInput is a well-formed captured-mail counterparty; tests perturb it.
func (e *dedupeEnv) ensureInput(ctx context.Context, t *testing.T, email, display, domain string) EnsureCounterpartyInput {
	t.Helper()
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction, source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'hi', 'inbound', 'gmail', $2, 'gmail:seed', 'connector:gmail')`,
			activityID, activityID.String())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return EnsureCounterpartyInput{
		Email: email, DisplayName: display, Domain: domain,
		OwnerID: e.rep, ActivityID: activityID,
		Source: "gmail:" + activityID.String(), CapturedBy: "connector:gmail",
	}
}

func TestEnsureCounterpartyReusesTheExactIncumbent(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// An unknown domain creates the CONTACT and opens the company
	// question. No company is invented from the domain label.
	first, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "carol@ensure.test", "Carol Example", "ensure.test"))
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if !first.ContactCreated {
		t.Fatalf("first ensure = %+v, want the contact created", first)
	}
	if first.CompanyID != nil {
		t.Fatalf("first ensure = %+v, want NO company from an unjudged domain", first)
	}
	if !first.TriagePending || first.TriageDomain != "ensure.test" {
		t.Fatalf("first ensure = %+v, want the triage question opened for ensure.test", first)
	}

	// The same address again: the exact tier lands on the incumbent — no twin
	// contact — and the still-open question is reported again rather than
	// re-recorded, so the sweep sees one domain and not two.
	second, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "carol@ensure.test", "Carol Example", "ensure.test"))
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second.ContactCreated || second.ContactID != first.ContactID {
		t.Fatalf("second ensure = %+v, want the incumbent %s reused", second, first.ContactID)
	}
	var questions int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM company_domain_disposition
			WHERE domain = 'ensure.test' AND status = 'pending'`).Scan(&questions)
	}); err != nil {
		t.Fatal(err)
	}
	if questions != 1 {
		t.Fatalf("%d open questions for ensure.test, want exactly 1", questions)
	}
}

func TestEnsureCounterpartyAttachesToACompanyThatAlreadyExists(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// A human typed the company in first. Capture must attach to it, not defer
	// a question about a domain the workspace has already answered by hand.
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Ensure Test GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "attach.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dave@attach.test", "Dave Example", "attach.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if res.TriagePending {
		t.Fatalf("ensure = %+v, want no question about a domain that already has a company", res)
	}
	if res.CompanyID == nil || res.CompanyID.UUID != ids.UUID(company.Id) {
		t.Fatalf("ensure = %+v, want the existing company %s attached", res, company.Id)
	}
	var employments int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM relationship
			WHERE contact_id = $1 AND kind = 'employment' AND is_current_primary`, res.ContactID).Scan(&employments)
	}); err != nil {
		t.Fatal(err)
	}
	if employments != 1 {
		t.Fatalf("%d employment edges, want 1 onto the existing company", employments)
	}
}

// The write shape on the capture path: an employment edge capture plants is a
// domain row, an audit_log row and an event_outbox row in ONE transaction, and
// a re-ensure that plants nothing writes none of the three.
//
// Both halves matter. Without the first, an employer appears on a contact with
// nothing recording who attached it or on what evidence — and an employer is a
// fact a human reads off the record and can be wrong about. Without the second,
// every repeated capture of the same mail would mint history for a write that
// did not happen, which is the failure ON CONFLICT DO NOTHING exists to avoid.
func TestCapturedEmploymentCarriesTheWriteShapeAndANoOpCarriesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: "Write Shape GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "writeshape.test", IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "erin@writeshape.test", "Erin Example", "writeshape.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	audits, events := ledgerRowsForRelationship(ctx, t, e, res.ContactID.UUID)
	if audits != 1 {
		t.Errorf("%d audit_log rows for the planted employment, want 1 — nothing records who attached this employer", audits)
	}
	if events != 1 {
		t.Errorf("%d event_outbox rows for the planted employment, want 1 — no consumer is told the edge appeared", events)
	}

	// The same mail again: both guards refuse the insert, so neither ledger
	// moves. Asserting the delta rather than a total is what makes this about
	// the no-op rather than about the first call.
	if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "erin@writeshape.test", "Erin Example", "writeshape.test")); err != nil {
		t.Fatalf("re-ensure: %v", err)
	}
	againAudits, againEvents := ledgerRowsForRelationship(ctx, t, e, res.ContactID.UUID)
	if againAudits != audits || againEvents != events {
		t.Errorf("a re-ensure that planted no edge still wrote history: audit %d→%d, outbox %d→%d",
			audits, againAudits, events, againEvents)
	}
}

// ledgerRowsForRelationship counts what the two ledgers hold for the employment
// edges of one contact. The join to relationship is what keeps this counting the
// edge's own rows rather than everything the ensure wrote.
func ledgerRowsForRelationship(ctx context.Context, t *testing.T, e *dedupeEnv, contactID ids.UUID) (audits, events int) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		// The outbox carries no audit_id column — the link lives inside the
		// envelope — so the event side matches on the edge id appearing in the
		// envelope text. Crude, and adequate here because the id is a v7 UUID
		// minted for this row: nothing else in a fresh fixture carries it.
		return tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM audit_log a
			          JOIN relationship r ON r.id = a.entity_id
			         WHERE a.entity_type = 'relationship' AND r.contact_id = $1 AND r.kind = 'employment'),
			       (SELECT count(*) FROM event_outbox o
			         WHERE EXISTS (
			           SELECT 1 FROM relationship r
			            WHERE r.contact_id = $1 AND r.kind = 'employment'
			              AND o.envelope::text LIKE '%' || r.id || '%'))`,
			contactID).Scan(&audits, &events)
	}); err != nil {
		t.Fatalf("counting the edge's ledger rows: %v", err)
	}
	return audits, events
}

func TestEnsureCounterpartyAsksNothingAboutConsumerMail(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// A consumer mailbox is answered by its own domain. Deferring it would buy
	// a crawl of gmail.com to learn what the list already says.
	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "carol@gmail.com", "Carol Example", "gmail.com"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !res.ContactCreated {
		t.Fatal("a consumer-mail counterparty is still a contact")
	}
	if res.CompanyID != nil || res.TriagePending {
		t.Fatalf("ensure = %+v, want no company and no question for consumer mail", res)
	}
	var questions int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM company_domain_disposition`).Scan(&questions)
	}); err != nil {
		t.Fatal(err)
	}
	if questions != 0 {
		t.Fatalf("%d disposition rows for consumer mail, want 0", questions)
	}
}

func TestEnsureCounterpartyQuarantinesImpersonationSuspects(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// A display name embedding an address on a DIFFERENT domain — the
	// classic spoof tell. The row still lands (hiding suspicious mail
	// would be worse), but quarantined for the review surface.
	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t,
		"boss@spoof.test", "ceo@real-corp.example", "spoof.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	var quarantined bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT quarantined_at IS NOT NULL FROM contact WHERE id = $1`, res.ContactID).Scan(&quarantined)
	}); err != nil {
		t.Fatal(err)
	}
	if !quarantined {
		t.Fatal("an embedded-foreign-address display name must land quarantined")
	}

	if _, err := e.store.EnsureCounterparty(ctx, EnsureCounterpartyInput{Email: "  "}); err == nil {
		t.Fatal("an empty email must refuse, not create")
	}
}

func TestApplySignatureFieldsLetsTheNewerStatementWinAndKeepsWhatItReplaced(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contact, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Sig Edge", Source: "manual",
		Emails: []ContactEmailInput{{Email: "sig@edge.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))

	// A title somebody typed. The contact's own signature, read from a message
	// dated after it, replaces it — that is the rule this pass now carries, and
	// the typed value is kept where a reader can put it back.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE contact SET title = 'Human-set CTO' WHERE id = $1`, contactID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	res, err := e.store.ApplySignatureFields(ctx, contactID, e.openSignatureSource(ctx, t), []SignatureField{
		{Name: "title", Value: "AI CTO", Evidence: "AI CTO", Confidence: 0.9},
		{Name: "", Value: "   ", Evidence: ""}, // a blank value is dropped before any write
	})
	if err != nil {
		t.Fatalf("ApplySignatureFields: %v", err)
	}
	if res.Applied != 1 || res.Skipped != 1 {
		t.Fatalf("apply = %+v, want 1 applied (the newer statement) / 1 skipped (the blank)", res)
	}
	var title string
	var superseded *string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT title FROM contact WHERE id = $1`, contactID).Scan(&title); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT superseded_value FROM contact_profile_field WHERE contact_id = $1 AND field = 'title'`,
			contactID).Scan(&superseded)
	}); err != nil {
		t.Fatal(err)
	}
	if title != "AI CTO" {
		t.Fatalf("title = %q — the contact's later statement must replace what was typed", title)
	}
	if superseded == nil || *superseded != "Human-set CTO" {
		t.Fatalf("superseded_value = %v, want the typed title: a replacement nobody can undo is a deletion", superseded)
	}

	// The phone lane is a LIST, so a number stated later takes the place of the
	// one it replaces and the replaced row is archived rather than dropped.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO contact_phone (contact_id, phone, phone_type, is_primary, position, source, captured_by, observed_at)
			VALUES ($1, '+49 30 9999999', 'work', true, 0, 'manual', 'human:test', now() - interval '30 days')`, contactID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, err = e.store.ApplySignatureFields(ctx, contactID, e.openSignatureSource(ctx, t), []SignatureField{
		{Name: "phone", Value: "+49 30 1234567", Evidence: "+49 30 1234567", Confidence: 0.8},
	})
	if err != nil {
		t.Fatalf("phone apply: %v", err)
	}
	if res.Applied != 1 {
		t.Fatalf("phone apply = %+v, want 1 applied: the signature states a newer number", res)
	}
	var live, archived string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT phone FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL`,
			contactID).Scan(&live); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT phone FROM contact_phone WHERE contact_id = $1 AND archived_at IS NOT NULL`,
			contactID).Scan(&archived)
	}); err != nil {
		t.Fatal(err)
	}
	if live != "+49301234567" || archived != "+49 30 9999999" {
		t.Fatalf("live = %q, archived = %q — the newer number rings and the older one stays recoverable", live, archived)
	}

	// A statement OLDER than the one on the record changes nothing: mail is
	// re-delivered, and a replay must not walk a value backwards.
	res, err = e.store.ApplySignatureFields(ctx, contactID, e.agedSignatureSource(ctx, t), []SignatureField{
		{Name: "title", Value: "Stale Title", Evidence: "Stale Title", Confidence: 0.9},
	})
	if err != nil {
		t.Fatalf("aged apply: %v", err)
	}
	if res.Applied != 0 || res.Skipped != 1 {
		t.Fatalf("aged apply = %+v, want 0 applied: an older statement never wins", res)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT title FROM contact WHERE id = $1`, contactID).Scan(&title)
	}); err != nil {
		t.Fatal(err)
	}
	if title != "AI CTO" {
		t.Fatalf("title = %q after an older statement, want the newer one to stand", title)
	}

	if res, err = e.store.ApplySignatureFields(ctx, contactID, e.openSignatureSource(ctx, t), nil); err != nil || res.Applied != 0 || res.Skipped != 0 {
		t.Fatalf("empty apply = %+v (err %v), want a zero no-op", res, err)
	}
}

func TestEnsureCounterpartySuppressedAddressStaysDead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO erasure_suppression (kind, value_hash)
			VALUES ('email', $1)`, storekit.SuppressionHash("dead@ensure.test"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dead@ensure.test", "Dead Address", "ensure.test"))
	if !errors.Is(err, ErrCounterpartySuppressed) {
		t.Fatalf("suppressed address = %v, want ErrCounterpartySuppressed", err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The signature pass over a real Postgres. A signature above a long company
// block and legal footer still lands. A rejected read is asked again until its
// cap, and rejected reads never chain a continuation.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// signatureFieldsBrain answers every signature read with the same fields.
type signatureFieldsBrain struct {
	fields []map[string]any
	calls  int
}

func (b *signatureFieldsBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.calls++
	reply, err := json.Marshal(map[string]any{"fields": b.fields})
	return model.Response{Text: string(reply)}, err
}

func signatureClaim(field, value string) map[string]any {
	return map[string]any{"field": field, "value": value, "evidence_snippet": value, "confidence": 0.9}
}

// seedSigner plants a contact and one inbound mail they sent.
func seedSigner(t *testing.T, e *Env, name, email, body string) ids.UUID {
	t.Helper()
	contact, mail := ids.NewV7(), ids.NewV7()
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'gmail:seed', 'connector:gmail')`, []any{contact, name}},
			{`INSERT INTO contact_email (contact_id, email, email_type, is_primary, source, captured_by)
			  VALUES ($1, $2, 'work', true, 'gmail:seed', 'connector:gmail')`, []any{contact, email}},
			{`INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source, captured_by)
			  VALUES ($1, 'email', 'Termin', $2, 'inbound', 'gmail', $3, 'gmail:seed', 'connector:gmail')`, []any{mail, body, mail.String()}},
			{`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, []any{mail, contact}},
			{`INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, $3, 'from')`, []any{mail, contact, email}},
		} {
			if _, err := tx.Exec(context.Background(), statement.sql, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return contact
}

func runSignaturePass(t *testing.T, e *Env, brain *signatureFieldsBrain) bool {
	t.Helper()
	enricher := compose.NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	filled, err := enricher.RunWorkspace(context.Background())
	if err != nil {
		t.Fatalf("the signature pass: %v", err)
	}
	return filled
}

// annaWebersMail is an Outlook reply: the signature, a company block and a legal
// footer, then a quoted appointment whose banner line the quote cut keeps.
const annaWebersMail = `From: Anna Weber <anna.weber@beispiel.example>
To: Peter Schmidt <peter@seller.example>

Hallo Herr Schmidt,

danke, der Termin am Donnerstag passt mir.

Mit freundlichen Grüßen

Anna Weber
Leiterin Einkauf
T +49 30 1234567
M +49 151 2345678
anna.weber@beispiel.example

Beispiel Handels GmbH
Musterstraße 12
10115 Berlin
Deutschland
www.beispiel-handels.example

Geschäftsführer: Max Mustermann, Erika Musterfrau
Sitz der Gesellschaft: Berlin, Amtsgericht Charlottenburg HRB 123456
Diese E-Mail enthält vertrauliche und/oder rechtlich geschützte Informationen.
Wenn Sie nicht der richtige Adressat sind, informieren Sie bitte sofort den Absender.
Das unerlaubte Kopieren dieser E-Mail ist nicht gestattet.
USt-IdNr.: DE123456789

-----Ursprünglicher Termin-----
Von: Peter Schmidt <peter@seller.example>
Gesendet: Montag, 2. November 2026 10:00
An: Anna Weber
Betreff: Jahresgespräch
Wann: Donnerstag, 5. November 2026 14:00-15:00
Wo: Microsoft Teams-Besprechung

Microsoft Teams-Besprechung
Klicken Sie hier, um an der Besprechung teilzunehmen <https://teams.microsoft.com/l/meetup-join/example>
`

func TestASignatureAboveALongFooterLandsItsTitleAndBothNumbers(t *testing.T) {
	e := Setup(t)
	anna := seedSigner(t, e, "Anna Weber", "anna.weber@beispiel.example", annaWebersMail)
	brain := &signatureFieldsBrain{fields: []map[string]any{
		signatureClaim("title", "Leiterin Einkauf"),
		signatureClaim("phone", "+49 30 1234567"),
		signatureClaim("phone", "+49 151 2345678"),
	}}
	runSignaturePass(t, e, brain)

	if title := e.WsScalar(t, `SELECT coalesce(title, '') FROM contact WHERE id = $1`, anna); title != "Leiterin Einkauf" {
		t.Errorf("title = %q, want the one under her name", title)
	}
	phones := e.WsScalar(t, `SELECT string_agg(phone, ',' ORDER BY phone COLLATE "C") FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL`, anna)
	if phones != "+491512345678,+49301234567" {
		t.Errorf("phones = %q, want both numbers her signature lists", phones)
	}
}

// signatureReadState is the read cursor one contact's row holds.
type signatureReadState struct {
	watermark string
	retries   int
}

func readSignatureState(t *testing.T, e *Env, contact ids.UUID) signatureReadState {
	t.Helper()
	var state signatureReadState
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT coalesce(activity_id::text, ''), retry_count
			  FROM contact_signature_enrich_state WHERE contact_id = $1`, contact).
			Scan(&state.watermark, &state.retries)
	}); err != nil {
		t.Fatalf("reading the signature state: %v", err)
	}
	return state
}

func TestARejectedReadIsAskedAgainUntilItsCapConsumesTheMail(t *testing.T) {
	e := Setup(t)
	bob := seedSigner(t, e, "Bob Contact", "bob@acme.example", "Sounds good.\n\nBest,\nBob Contact\nCTO\nAcme GmbH")
	mail := e.WsScalar(t, `SELECT activity_id::text FROM activity_link WHERE contact_id = $1`, bob)
	// The snippet is nowhere in the signature, so the evidence gate refuses it.
	brain := &signatureFieldsBrain{fields: []map[string]any{signatureClaim("title", "Chief Technology Officer")}}

	for attempt := 1; attempt < contacts.SignatureReadAttempts; attempt++ {
		runSignaturePass(t, e, brain)
		state := readSignatureState(t, e, bob)
		if state.watermark != "" || state.retries != attempt {
			t.Fatalf("after rejected read %d the state is %+v, want no watermark and the retry counted", attempt, state)
		}
		runSignaturePass(t, e, brain)
		if brain.calls != attempt {
			t.Fatalf("retry ignored its backoff: %d calls after attempt %d", brain.calls, attempt)
		}
		e.WsExec(t, `UPDATE contact_signature_enrich_state SET attempted_at = '-infinity' WHERE contact_id = $1`, bob)
	}
	runSignaturePass(t, e, brain)
	want := signatureReadState{watermark: mail, retries: contacts.SignatureReadAttempts}
	if state := readSignatureState(t, e, bob); state != want {
		t.Fatalf("after the capped read the state is %+v, want the mail consumed %+v", state, want)
	}
	runSignaturePass(t, e, brain)
	if brain.calls != contacts.SignatureReadAttempts {
		t.Errorf("the model was asked %d times, want %d and none after the cap", brain.calls, contacts.SignatureReadAttempts)
	}
}

func TestAPassOfOnlyRejectedReadsQueuesNoContinuation(t *testing.T) {
	e := Setup(t)
	signers := make([]ids.UUID, 0, 100)
	for i := range 100 {
		name := "Signer " + strconv.Itoa(i)
		signers = append(signers, seedSigner(t, e, name, fmt.Sprintf("signer%d@buyer.example", i),
			"Sounds good.\n\nBest,\n"+name+"\nCTO"))
	}
	brain := &signatureFieldsBrain{fields: []map[string]any{signatureClaim("title", "Chief Technology Officer")}}
	if runSignaturePass(t, e, brain) {
		t.Error("a full pass that moved nobody asked for a continuation")
	}
	states := make([]signatureReadState, 0, len(signers))
	for _, signer := range signers {
		states = append(states, readSignatureState(t, e, signer))
	}
	retried := signatureReadState{retries: 1}
	if slices.ContainsFunc(states, func(state signatureReadState) bool { return state != retried }) {
		t.Errorf("read states = %v, want every read refused and kept for a retry", states)
	}
}

func TestSomebodyElsesSignatureIsReadOnce(t *testing.T) {
	e := Setup(t)
	bob := seedSigner(t, e, "Bob Contact", "bob@acme.example", "Forwarding this.\n\nBest,\nKim Vendor\nCEO, Vendor Ltd")
	mail := e.WsScalar(t, `SELECT activity_id::text FROM activity_link WHERE contact_id = $1`, bob)
	brain := &signatureFieldsBrain{}

	runSignaturePass(t, e, brain)
	want := signatureReadState{watermark: mail}
	if state := readSignatureState(t, e, bob); state != want {
		t.Fatalf("the state is %+v, want the mail consumed without a retry %+v", state, want)
	}
	if brain.calls != 0 {
		t.Errorf("the model was asked %d times about a signature that is not the contact's", brain.calls)
	}
}

func TestAReopenedSignatureGetsANewRetryBudget(t *testing.T) {
	e := Setup(t)
	bob := seedSigner(t, e, "Bob Contact", "bob@acme.example", "Best,\nBob Contact\nCTO")
	brain := &signatureFieldsBrain{fields: []map[string]any{signatureClaim("title", "Chief Technology Officer")}}
	runSignaturePass(t, e, brain)
	ctx := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:signature-reopen", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := contacts.QueueSignatureEventTx(ctx, tx, []ids.ContactID{ids.From[ids.ContactKind](bob)}, contacts.SignatureRequest{Reopen: true, Live: true})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runSignaturePass(t, e, brain)
	if state := readSignatureState(t, e, bob); brain.calls != 2 || state.retries != 1 {
		t.Fatalf("reopened read: calls %d, state %+v; want an immediate first retry of the new work", brain.calls, state)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Bob's own mail can carry Alice's signature beside Bob's address. Nothing in
// that block is Bob's: not the name, the title or the number.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	aliceBlock = "Sounds good.\n\nBest regards\nAlice Example\nFounder\n+49 30 5550100\n"
	aliceName  = `"name":{"family_name":"Example","given_names":"Alice","middle":"","order":"given_first","evidence_snippet":"Alice Example","confidence":0.9}}`
	aliceReply = `{"fields":[{"field":"title","value":"Founder","evidence_snippet":"Founder","confidence":0.9},` +
		`{"field":"phone","value":"+49 30 5550100","evidence_snippet":"+49 30 5550100","confidence":0.9}],` + aliceName
	bobReply = `{"fields":[{"field":"title","value":"Buyer","evidence_snippet":"Buyer","confidence":0.9}],` +
		`"name":{"family_name":"Contact","given_names":"Bob","middle":"","order":"given_first","evidence_snippet":"Bob Contact","confidence":0.9}}`
)

// seedVerdictContact plants Bob as the counterparty verdict creates him: the
// display his own header entry gave, and one mail of his with the given body.
func seedVerdictContact(t *testing.T, e *integration.Env, display, body string) ids.UUID {
	t.Helper()
	contact := seedEnrichContact(t, e, "bob@example.org", body)
	e.WsExec(t, `UPDATE contact SET full_name = $2, captured_by = $3 WHERE id = $1`, contact, display, contacts.CaptureVerdictActor)
	return contact
}

// assertSignatureWrites checks the display, the agent:enrich renames and the
// title and phone rows the contact carries.
func assertSignatureWrites(t *testing.T, e *integration.Env, contact ids.UUID, display string, renames, titles, phones int) {
	t.Helper()
	full := e.WsScalar(t, `SELECT full_name FROM contact WHERE id = $1`, contact)
	gotRenames := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1
		AND after ? 'full_name' AND actor_id = 'agent:enrich'`, contact)
	gotTitles := e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND title IS NOT NULL`, contact)
	gotPhones := len(liveNumbers(t, e, contact))
	if full != display || gotRenames != renames || gotTitles != titles || gotPhones != phones {
		t.Errorf("display %q, agent:enrich renames %d, titles %d, phones %d; want %q, %d, %d, %d",
			full, gotRenames, gotTitles, gotPhones, display, renames, titles, phones)
	}
}

func TestSignatureEnrichReadsOnlyTheContactsOwnSignature(t *testing.T) {
	for name, tc := range map[string]struct {
		display, body, reply string
		titles, phones       int
	}{
		"another participant's block beside only the contact's address": {"Bob", aliceBlock + "bob@example.org", aliceReply, 0, 0},
		"another participant's block with both addresses":               {"Bob", aliceBlock + "alice@example.com\nbob@example.org", aliceReply, 0, 0},
		"the contact's own block keeps the header name":                 {"Bob Contact", "Thanks.\n\nBest regards\nBob Contact\nBuyer", bobReply, 1, 0},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			contact := seedVerdictContact(t, e, tc.display, tc.body)
			runNamePass(t, e, tc.reply)
			assertSignatureWrites(t, e, contact, tc.display, 0, tc.titles, tc.phones)
		})
	}
}

func TestTheRepairTakesBackAnotherParticipantsSignature(t *testing.T) {
	e := integration.Setup(t)
	contact := seedVerdictContact(t, e, "Bob", aliceBlock+"bob@example.org")
	name := func(field, value string) contacts.SignatureField {
		f := signatureField(field, value)
		f.Evidence, f.Display = "Alice Example", "Alice Example"
		return f
	}
	// The pass as it wrote before it checked whose name it read.
	if _, err := e.Contacts.ApplySignatureFields(principal.SystemActing(e.Admin(), "agent:enrich"), ids.From[ids.ContactKind](contact),
		onlyMailOf(t, e, contact), []contacts.SignatureField{name(contacts.FieldFirstName, "Alice"), name(contacts.FieldLastName, "Example"),
			signatureField("title", "Founder"), phoneField("+49 30 5550100")}, true); err != nil {
		t.Fatal(err)
	}
	assertSignatureWrites(t, e, contact, "Alice Example", 1, 1, 1)

	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, signatureWindowDataTask())

	assertSignatureWrites(t, e, contact, "Bob", 1, 0, 0)
}

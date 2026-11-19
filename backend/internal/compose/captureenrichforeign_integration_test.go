// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A contact's own mail can carry another participant's signature. Nothing in
// that block is theirs. Their own signature is read under any spelling of
// their name: accents, initials, nicknames, hyphens and run-together addresses.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	aliceBlock  = "Sounds good.\n\nBest regards\nAlice Example\nFounder\n+49 30 5550100\n"
	aliceFields = `{"fields":[{"field":"title","value":"Founder","evidence_snippet":"Founder","confidence":0.9},` +
		`{"field":"phone","value":"+49 30 5550100","evidence_snippet":"+49 30 5550100","confidence":0.9}]`
	aliceReply = aliceFields + `,"name":{"family_name":"Example","given_names":"Alice","middle":"","order":"given_first",` +
		`"evidence_snippet":"Alice Example","confidence":0.9}}`
)

type ownRead struct{ name, header, email, line, given, middle, family, order string }

// ownReads are signatures that are the contact's own under another spelling.
var ownReads = []ownRead{
	{"accents", "Nguyen Van An", "an.nguyen@example.vn", "Nguyễn Văn An", "An", "Văn", "Nguyễn", "family_first"},
	{"initial", "J. Smith", "office@example.com", "John Smith", "John", "", "Smith", "given_first"},
	{"nickname", "Bob Smith", "sales@example.com", "Robert Smith", "Robert", "", "Smith", "given_first"},
	{"hyphen", "Anna Lena Schmidt", "info@example.de", "Anna-Lena Schmidt", "Anna-Lena", "", "Schmidt", "given_first"},
	{"compact address", "Greven Ebusiness", "mgreven@greven.example", "Marcus Greven", "Marcus", "", "Greven", "given_first"},
}

// reply reads the title CTO and the name printed on the read's first line.
func (r ownRead) reply() string {
	line, _, _ := strings.Cut(r.line, "\n")
	return fmt.Sprintf(`{"fields":[{"field":"title","value":"CTO","evidence_snippet":"CTO","confidence":0.9}],`+
		`"name":{"family_name":%q,"given_names":%q,"middle":%q,"order":%q,"evidence_snippet":%q,"confidence":0.9}}`,
		r.family, r.given, r.middle, r.order, line)
}

// seedVerdictContact plants a contact as the counterparty verdict creates it:
// the display their own header entry gave, and one mail of theirs.
func seedVerdictContact(t *testing.T, e *integration.Env, email, display, body string) ids.UUID {
	t.Helper()
	contact := seedEnrichContact(t, e, email, body)
	e.WsExec(t, `UPDATE contact SET full_name = $2, captured_by = $3 WHERE id = $1`, contact, display, contacts.CaptureVerdictActor)
	return contact
}

// signatureWrites is the display, the agent:enrich renames, and the title and
// phone rows the contact carries.
func signatureWrites(t *testing.T, e *integration.Env, contact ids.UUID) string {
	t.Helper()
	return fmt.Sprintf("%s|%d|%d|%d", e.WsScalar(t, `SELECT full_name FROM contact WHERE id = $1`, contact),
		e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1
			AND after ? 'full_name' AND actor_id = 'agent:enrich'`, contact),
		e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND title IS NOT NULL`, contact), len(liveNumbers(t, e, contact)))
}

func TestSignatureEnrichReadsOnlyTheContactsOwnSignature(t *testing.T) {
	for name, tc := range map[string]struct{ body, reply string }{
		"another signer's block beside only the contact's address": {aliceBlock + "bob@example.org", aliceReply},
		"another signer's block with both addresses":               {aliceBlock + "alice@example.com\nbob@example.org", aliceReply},
		"only the contact's address and no name read":              {"Thanks.\n\nFounder\n+49 30 5550100\nbob@example.org", aliceFields + "}"},
		"a reply header above another signer's block":              {"Thanks.\n\nBob <bob@example.org> wrote:\nBest\nAlice Example\nFounder\n+49 30 5550100", aliceFields + "}"},
		"a forwarded signature": {"FYI\n\n---------- Forwarded message ---------\nFrom: Alice Example <alice@example.com>\n" +
			"To: bob@example.org\n\n" + aliceBlock, aliceReply},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			contact := seedVerdictContact(t, e, "bob@example.org", "Bob", tc.body)
			runNamePass(t, e, tc.reply)
			if got := signatureWrites(t, e, contact); got != "Bob|0|0|0" {
				t.Errorf("display|renames|titles|phones = %s; another participant's signature reached the contact", got)
			}
		})
	}
	colleague := ownRead{
		"a colleague's address beside their own", "Bob Contact", "bob@example.org",
		"Bob Contact\nbob@example.org\nCc: carol@example.org", "Bob", "", "Contact", "given_first",
	}
	for _, tc := range append(ownReads, colleague) {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			contact := seedVerdictContact(t, e, tc.email, tc.header, "Thanks.\n\nBest regards\n"+tc.line+"\nCTO\n"+tc.email)
			runNamePass(t, e, tc.reply())
			if title := contactTitle(t, e, contact); title == nil {
				t.Errorf("no title read: the contact's own signature %q was refused for %q", tc.line, tc.header)
			}
		})
	}
}

func TestTheRepairTakesBackAnotherParticipantsSignatureAndKeepsTheirOwn(t *testing.T) {
	e := integration.Setup(t)
	apply := func(contact ids.UUID, given, family, display string) {
		name := func(field, value string) contacts.SignatureField {
			f := signatureField(field, value)
			f.Evidence, f.Display = display, display
			return f
		}
		if _, err := e.Contacts.ApplySignatureFields(principal.SystemActing(e.Admin(), "agent:enrich"), ids.From[ids.ContactKind](contact),
			onlyMailOf(t, e, contact), []contacts.SignatureField{
				name(contacts.FieldFirstName, given), name(contacts.FieldLastName, family),
				signatureField("title", "CTO"), phoneField("+49 30 5550100"),
			}, true); err != nil {
			t.Fatal(err)
		}
	}
	// The pass as it wrote before it checked whose name it read.
	bob := seedVerdictContact(t, e, "bob@example.org", "Bob", aliceBlock+"bob@example.org")
	apply(bob, "Alice", "Example", "Alice Example")
	kept := map[string]ids.UUID{}
	for _, tc := range ownReads {
		kept[tc.name] = seedVerdictContact(t, e, tc.email, tc.header, "Best regards\n"+tc.line+"\nCTO\n+49 30 5550100")
		apply(kept[tc.name], tc.given, tc.family, tc.line)
	}

	runMachineAddressTask(principal.SystemActing(e.Admin(), "system:datatask-test"), t, e, signatureWindowDataTask())

	if got := signatureWrites(t, e, bob); got != "Bob|1|0|0" {
		t.Errorf("display|renames|titles|phones = %s, want Alice's name, title and number taken back", got)
	}
	for name, contact := range kept {
		if title := contactTitle(t, e, contact); title == nil || len(liveNumbers(t, e, contact)) != 1 {
			t.Errorf("%s: the repair took back the contact's own signature", name)
		}
	}
}

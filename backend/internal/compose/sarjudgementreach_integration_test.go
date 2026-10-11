// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Which judgements the access export hands a subject when no activity_link ties
// the judged message to them. Authorship earns one, being a recipient does not,
// and an address another live contact now holds earns nothing.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	aliceAddress = "alice.kessler@buyer.test"
	bobAddress   = "bob.ferrer@buyer.test"
)

// seedContactWithAddress creates a contact through the real contact writer, so
// the address is stored the way the export's address arm reads it.
func seedContactWithAddress(t *testing.T, e *integration.Env, name, address string) ids.UUID {
	t.Helper()
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

func assembleExport(t *testing.T, e *integration.Env, contact ids.UUID) privacy.SARPackage {
	t.Helper()
	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("AssembleSAR → %v", err)
	}
	return pkg
}

// judgedInboundFrom writes an inbound mail from the address, filed only to a
// company, and judges it through the real verdict writer.
func judgedInboundFrom(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	employer := e.SeedCompany(t, "Buyer GmbH "+ids.NewV7().String(), nil)
	reply := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, direction, subject, body, counterparty_email,
		                              occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Re: terms', 'Sounds good.', $2, now(), 'manual', 'human:seed')`,
		reply, address)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, reply, employer)
	applied, err := activities.NewStore(e.DB()).SetReplyVerdict(e.Admin(), reply, activities.ReplyVerdictPositive, "test-model")
	if err != nil || !applied {
		t.Fatalf("judging the reply: applied=%v err=%v", applied, err)
	}
	return reply
}

func TestTheAccessExportCarriesStageEvidenceFromAMeetingTheSubjectSpokeIn(t *testing.T) {
	e := integration.Setup(t)
	contact := seedContactWithAddress(t, e, "Alice Kessler", aliceAddress)
	spoken := seedTranscript(t, e, "1: Tom: Where are we on terms?\n2: "+quotedAcceptance)
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id)
		VALUES ($1, $2, 'from', $3)`, ids.NewV7(), spoken, contact)
	evidenceQuoting(t, e, spoken, quotedAcceptance)

	// The subject only attended this one: somebody else's words are not theirs.
	const strangerQuote = "Bob Ferrer: The terms work for us, please send the order form."
	attended := seedTranscript(t, e, "1: Tom: Terms?\n2: "+strangerQuote)
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id)
		VALUES ($1, $2, 'attendee', $3)`, ids.NewV7(), attended, contact)
	evidenceQuoting(t, e, attended, strangerQuote)

	pkg := assembleExport(t, e, contact)
	if len(pkg.StageEvidence) != 1 || pkg.StageEvidence[0]["snippet"] != quotedAcceptance {
		t.Errorf("the export must carry exactly the claim read from the meeting the subject spoke in: %+v", pkg.StageEvidence)
	}
}

func TestTheAccessExportGivesTheReplyVerdictToItsAuthorAndNotToACcRecipient(t *testing.T) {
	e := integration.Setup(t)
	alice := seedContactWithAddress(t, e, "Alice Kessler", aliceAddress)
	bob := seedContactWithAddress(t, e, "Bob Ferrer", bobAddress)
	reply := judgedInboundFrom(t, e, aliceAddress)
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', $3), ($4, $2, 'cc', $5)`,
		ids.NewV7(), reply, aliceAddress, ids.NewV7(), bobAddress)

	if got := assembleExport(t, e, alice).ReplyJudgements; len(got) != 1 || got[0]["verdict"] != activities.ReplyVerdictPositive {
		t.Errorf("the export omits the verdict on the reply its subject wrote: %+v", got)
	}
	if got := assembleExport(t, e, bob).ReplyJudgements; len(got) != 0 {
		t.Errorf("a Cc recipient's export carries the verdict on somebody else's reply: %+v", got)
	}
}

func TestTheAccessExportWithholdsTheSnippetOfALimitedMessage(t *testing.T) {
	e := integration.Setup(t)
	contact := seedContactWithAddress(t, e, "Alice Kessler", aliceAddress)
	limited := seedTranscript(t, e, "1: Tom: Where are we on terms?\n2: "+quotedAcceptance)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, limited, contact)
	evidenceQuoting(t, e, limited, quotedAcceptance)
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, limited)

	got := assembleExport(t, e, contact).StageEvidence
	if len(got) != 1 {
		t.Fatalf("the export must still disclose the claim on a limited message: %+v", got)
	}
	if got[0]["snippet"] != nil || got[0]["content_disclosed"] != false {
		t.Errorf("the export hands out a quotation of a message whose body it withholds: %+v", got[0])
	}
}

func TestTheAccessExportDoesNotFollowAnAddressAnotherLiveContactHolds(t *testing.T) {
	e := integration.Setup(t)
	alice := seedContactWithAddress(t, e, "Alice Kessler", aliceAddress)
	// Alice used to hold the address; Carol holds it now.
	e.WsExec(t, `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, alice)
	seedContactWithAddress(t, e, "Carol Weber", aliceAddress)
	judgedInboundFrom(t, e, aliceAddress)

	if got := assembleExport(t, e, alice).ReplyJudgements; len(got) != 0 {
		t.Errorf("the export followed an address another live contact holds into their mail: %+v", got)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the access export concludes about messages no activity_link ties to the
// subject. One is a meeting they only attended, one a reply filed to their
// employer alone.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const subjectAddress = "mara.kessler@buyer.test"

// seedSubjectWithAddress creates the subject through the real contact writer,
// so the address is stored the way the export's address arm reads it.
func seedSubjectWithAddress(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Mara Kessler", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: subjectAddress, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the subject: %v", err)
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

func TestTheAccessExportCarriesStageEvidenceFromAMeetingTheSubjectOnlyAttended(t *testing.T) {
	e := integration.Setup(t)
	contact := seedSubjectWithAddress(t, e)
	attended := seedTranscript(t, e, "1: Tom: Where are we on terms?\n2: "+quotedAcceptance)
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id)
		VALUES ($1, $2, 'attendee', $3)`, ids.NewV7(), attended, contact)
	evidenceQuoting(t, e, attended, quotedAcceptance)

	// A meeting with somebody else's words: the wider reach must not sweep it in.
	const strangerQuote = "Bob Ferrer: The terms work for us, please send the order form."
	stranger := seedTranscript(t, e, "1: Tom: Terms?\n2: "+strangerQuote)
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'attendee', 'bob.ferrer@elsewhere.test')`, ids.NewV7(), stranger)
	evidenceQuoting(t, e, stranger, strangerQuote)

	pkg := assembleExport(t, e, contact)
	if len(pkg.StageEvidence) != 1 || pkg.StageEvidence[0]["snippet"] != quotedAcceptance {
		t.Errorf("the export must carry exactly the claim read from the meeting the subject attended: %+v", pkg.StageEvidence)
	}
}

func TestTheAccessExportCarriesTheVerdictOnAReplyFiledOnlyToTheSubjectsEmployer(t *testing.T) {
	e := integration.Setup(t)
	contact := seedSubjectWithAddress(t, e)
	employer := e.SeedCompany(t, "Buyer GmbH", nil)
	reply := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, direction, subject, body, counterparty_email,
		                              occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Re: terms', 'Sounds good.', $2, now(), 'manual', 'human:seed')`,
		reply, subjectAddress)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, reply, employer)
	applied, err := activities.NewStore(e.DB()).SetReplyVerdict(e.Admin(), reply, activities.ReplyVerdictPositive, "test-model")
	if err != nil || !applied {
		t.Fatalf("judging the reply: applied=%v err=%v", applied, err)
	}

	pkg := assembleExport(t, e, contact)
	if len(pkg.ReplyJudgements) != 1 || pkg.ReplyJudgements[0]["verdict"] != activities.ReplyVerdictPositive {
		t.Errorf("the export omits the verdict on the subject's reply filed only to their employer: %+v", pkg.ReplyJudgements)
	}
}

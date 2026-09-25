// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

// Withholding: a stored finding outlives the GRANTS it was written under, not
// only the records. The scan runs while the reader is on the thread, the author
// narrows the message's audience, and the card must stop printing the subject
// and the sentence it quoted — while keeping the advice, because the reader may
// still legitimately know the message is there.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// quoting is a finding citing one activity with the two content-bearing fields
// the scan stores: the message's subject and a verbatim extract of its body.
func quoting(id openapi_types.UUID) crmcontracts.Company360Suggestion {
	name, quote := "Renewal terms", "we will not be renewing at this price"
	return crmcontracts.Company360Suggestion{
		Fingerprint: "fp",
		Evidence: []crmcontracts.CompanyBriefEvidence{{
			EntityType: crmcontracts.CompanyBriefEvidenceEntityTypeActivity,
			EntityId:   id,
			Name:       &name,
			Quote:      &quote,
		}},
	}
}

func TestACitationTheReaderMayNoLongerReadLosesItsWords(t *testing.T) {
	narrowed := activityID(1)
	findings := []crmcontracts.Company360Suggestion{quoting(narrowed)}

	blankUnreadable(findings, standingSet())

	row := findings[0].Evidence[0]
	if row.Name != nil {
		t.Errorf("the citation still carries the subject %q after the message's audience excluded this reader", *row.Name)
	}
	if row.Quote != nil {
		t.Errorf("the citation still quotes %q after the message's audience excluded this reader", *row.Quote)
	}
}

// The card stays. An audience narrowing is not a retraction: the record still
// exists and the reader may still be entitled to know it does, so dropping the
// advice would lose guidance they are owed.
func TestWithholdingTheWordsDoesNotRetractTheFinding(t *testing.T) {
	findings := []crmcontracts.Company360Suggestion{quoting(activityID(1))}

	blankUnreadable(findings, standingSet())

	if len(findings) != 1 {
		t.Fatalf("%d findings survived the withholding; want the advice kept", len(findings))
	}
}

func TestACitationTheReaderMayStillReadKeepsItsWords(t *testing.T) {
	open := activityID(2)
	findings := []crmcontracts.Company360Suggestion{quoting(open)}

	blankUnreadable(findings, standingSet(open))

	row := findings[0].Evidence[0]
	if row.Name == nil || row.Quote == nil {
		t.Errorf("a citation this reader may still read lost its words: name %v, quote %v", row.Name, row.Quote)
	}
}

// Only the arm the answer is about. A deal's name and a signal's sentence are
// admitted by their own records' gates, and an activity's answer says nothing
// about either.
func TestANonActivityCitationKeepsItsWordsWhateverTheActivityAnswerSays(t *testing.T) {
	name, quote := "Renewal 2027", "expansion agreed at the QBR"
	findings := []crmcontracts.Company360Suggestion{{
		Fingerprint: "fp",
		Evidence: []crmcontracts.CompanyBriefEvidence{{
			EntityType: crmcontracts.CompanyBriefEvidenceEntityTypeDeal,
			EntityId:   activityID(3),
			Name:       &name,
			Quote:      &quote,
		}},
	}}

	blankUnreadable(findings, standingSet())

	row := findings[0].Evidence[0]
	if row.Name == nil || row.Quote == nil {
		t.Errorf("a deal citation was blanked by the activity content answer: name %v, quote %v", row.Name, row.Quote)
	}
}

// One finding can rest on several messages, and a narrowing reaches one of
// them. The reader keeps what they may still check the claim against.
func TestOnlyTheNarrowedCitationOfAFindingIsBlanked(t *testing.T) {
	narrowed, open := activityID(1), activityID(2)
	finding := quoting(narrowed)
	finding.Evidence = append(finding.Evidence, quoting(open).Evidence...)
	findings := []crmcontracts.Company360Suggestion{finding}

	blankUnreadable(findings, standingSet(open))

	if findings[0].Evidence[0].Quote != nil {
		t.Errorf("the narrowed citation kept its quote")
	}
	if findings[0].Evidence[1].Quote == nil {
		t.Errorf("the citation this reader may still read lost its quote")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// CASE 2 — When somebody hands you a business card.
//
// The prompt, verbatim from the run, lowercase "crm" and all:
//
//	I met her at a conference. Tech Sauce in Bangkok actually. Add her to
//	the crm
//
// The point of the scenario is the duplicate check, not the OCR. Reading the
// photo is the assistant's job and Margince never sees an image, so the card
// arrives here as FIELDS.
//
// The two lanes are deliberately different products and this suite pins both:
// an email address belongs to one person, so a second create on the same
// address is REFUSED; a phone number belongs to a switchboard, so a second
// create sharing one LANDS and files a candidate for review. Collapsing them
// would either lose real people or let real duplicates through.
//
// NOT covered here:
//
//   - Criterion 4, the assistant REPORTING the flag. That is a model question.
//   - Criterion 5, the title conflict, which no run has yet exercised — no test
//     card has carried a title differing from a stored one, and the choice has
//     nowhere to be expressed on this surface today.
//   - The location trigger. It needs the user's whereabouts, which the
//     assistant's viewer refuses to share; the blocker is not ours.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createdFromCard is what create_record answers: the record, plus whatever it
// filed for review on the way.
//
// Declared here rather than reusing a product type because agents.createdRecord
// is unexported — which is correct, it is a wire shape and not an API. What a
// scenario needs is the two members it asserts on.
type createdFromCard struct {
	ID                  ids.UUID                    `json:"id"`
	DuplicateCandidates []agents.DuplicateCandidate `json:"duplicate_candidates"`
}

// The card, as the assistant reads it off the photo.
const (
	cardName  = "Lucy Vo"
	cardEmail = "lucy.vo@terralogic.test"
	cardPhone = "+842838256789"
	cardTitle = "Cybersecurity Sales Manager"
)

// createFromCard is one "add her to the crm", with whatever the card carried.
func (s *scenario) createFromCard(t *testing.T, fields map[string]any) createdFromCard {
	t.Helper()
	got := s.MCP.CallOK(t, "create_record", map[string]any{
		"record_type": "person", "fields": fields,
	})
	var created createdFromCard
	got.JSON(t, &created)
	return created
}

// cardFields is the card as tool arguments.
//
// Only the ADDRESS varies between the cards this case sends, and that is the
// scenario rather than a convenience: one human, two cards, the same name and
// the same company switchboard printed on both. The address is what makes the
// two lanes differ — an email is a real key and a switchboard is not.
func cardFields(email string) map[string]any {
	return map[string]any{
		"full_name": cardName,
		"title":     cardTitle,
		"emails":    []map[string]any{{"email": email, "is_primary": true}},
		"phones":    []map[string]any{{"phone": cardPhone, "is_primary": true}},
	}
}

// TestCase2TheSamePersonIsNotCreatedTwice pins criteria 1 and 2.
//
// The system refuses; it does not rely on the assistant being careful. An
// assistant that checked first is good behaviour and not a guarantee — the same
// card sent twice by a less careful one must still not produce two Lucys.
//
// Criterion 2 is the half that makes the refusal usable: "already exists" is
// not enough, it has to say WHO, so the user can go and look.
func TestCase2TheSamePersonIsNotCreatedTwice(t *testing.T) {
	s := boot(t, scopesReadWrite)

	first := s.createFromCard(t, cardFields(cardEmail))
	if first.ID.IsZero() {
		t.Fatalf("case 2: the first card did not create a person")
	}

	refusal := s.MCP.CallRefused(t, "create_record", map[string]any{
		"record_type": "person", "fields": cardFields(cardEmail),
	})
	if !strings.Contains(strings.ToLower(refusal), "already exists") {
		t.Fatalf("case 2 criterion 1: a second card for the same address was not refused as a "+
			"duplicate:\n%s", refusal)
	}
	// Criterion 2: the refusal names the address that collided, which is what
	// a user searches on to find the record already there.
	if !strings.Contains(strings.ToLower(refusal), strings.ToLower(cardEmail)) {
		t.Fatalf("case 2 criterion 2: the refusal does not name what already exists, so a user is "+
			"told no and given nowhere to look:\n%s", refusal)
	}

	if people := s.countRows(t,
		`SELECT count(*) FROM person WHERE full_name = $1 AND archived_at IS NULL`,
		cardName); people != 1 {
		t.Fatalf("case 2 criterion 1: the workspace holds %d people called %s after the same card "+
			"was added twice", people, cardName)
	}
}

// TestCase2ASharedPhoneLandsAndIsFiledForReview pins criterion 3.
//
// A switchboard belongs to many people, so this must NOT refuse — refusing
// would lose a real second contact at a company whose number everyone shares.
// It lands, and a possible duplicate is filed rather than lost.
func TestCase2ASharedPhoneLandsAndIsFiledForReview(t *testing.T) {
	s := boot(t, scopesReadWrite)

	first := s.createFromCard(t, cardFields(cardEmail))

	// The same human, a second card, a different address — and the company
	// switchboard both cards print.
	second := s.createFromCard(t, cardFields("lucy.vo@terralogic-vn.test"))
	if second.ID.IsZero() {
		t.Fatalf("case 2 criterion 3: a card sharing only a switchboard number was refused; a " +
			"shared number is not a shared identity and refusing loses a real contact")
	}
	if second.ID == first.ID {
		t.Fatalf("case 2 criterion 3: the second card returned the first person's id rather than " +
			"creating a record")
	}

	// The candidate travels on the CREATE'S OWN answer. There is no dedupe tool
	// on this surface and the review queue is cookie-only, so if the create
	// does not report it, an assistant has no way to learn a human was asked.
	if len(second.DuplicateCandidates) == 0 {
		t.Fatalf("case 2 criterion 3: the create filed no possible-duplicate candidate, so a second " +
			"record for the same human sits in the CRM with nobody asked about it")
	}
	named, onThePhone := false, false
	for _, candidate := range second.DuplicateCandidates {
		if candidate.OtherRecordID == first.ID.String() {
			named = true
		}
		if candidate.Confidence <= 0 {
			t.Fatalf("case 2 criterion 3: a candidate was filed with confidence %v, which tells a "+
				"reviewer nothing about how sure the match is", candidate.Confidence)
		}
		// WHICH axis they met on. Both cards also carry the same name, and the
		// name lane files a candidate of its own — so a test happy with "some
		// candidate exists" passes with the phone lane deleted entirely, which
		// is the lane this scenario is about.
		for _, evidence := range candidate.Evidence {
			if evidence.Field != "phone" {
				continue
			}
			onThePhone = true
			if evidence.Left != cardPhone || evidence.Right != cardPhone {
				t.Fatalf("case 2 criterion 3: the phone evidence compares %q with %q, and both "+
					"cards print %q — a reviewer reads these two values out before merging",
					evidence.Left, evidence.Right, cardPhone)
			}
		}
	}
	if !named {
		t.Fatalf("case 2 criterion 3: the candidate does not name the record already in the "+
			"workspace (%s); a reviewer offered a merge has to be told which is which", first.ID)
	}
	if !onThePhone {
		t.Fatalf("case 2 criterion 3: no candidate cites the shared PHONE. The two cards also " +
			"share a name, and the name lane files its own candidate — so this scenario only " +
			"tests what it claims when the phone axis is the one that fired")
	}
}

// TestCase2ATagIsAppliedByNameInOneStep pins criterion 6.
//
// By NAME, in one call: "Add tag: Champion" is one act to the person asking,
// and an assistant that had to look the id up first would be two round trips
// deep in a conversation about a business card.
//
// The word has to exist. The vocabulary is Admin and Ops's to extend, so the
// name is a reference to a word somebody already chose — this test creates it
// as an admin first, which is the same order a real workspace works in.
func TestCase2ATagIsAppliedByNameInOneStep(t *testing.T) {
	s := boot(t, scopesReadWrite)
	person := s.createFromCard(t, cardFields(cardEmail))

	const word = "Tech Sauce Bangkok 2026"
	s.seedTag(t, word)

	got := s.MCP.CallOK(t, "apply_tag", map[string]any{
		"tag_name": word, "record_type": "person", "record_id": person.ID.String(),
	})
	var applied agents.TagAppliedResult
	got.JSON(t, &applied)

	if !applied.Applied {
		t.Fatalf("case 2 criterion 6: applying a tag by name answered applied=false:\n%s", got.Text)
	}
	if applied.TagID.IsZero() {
		t.Fatalf("case 2 criterion 6: the tag was applied without naming the word's id, so a " +
			"caller cannot refer to it again")
	}
	if n := s.countRows(t, `SELECT count(*) FROM taggable WHERE tag_id = $1 AND entity_type = 'person' AND entity_id = $2`,
		applied.TagID, person.ID); n != 1 {
		t.Fatalf("case 2 criterion 6: the tag reports applied and %d rows attach it to the person", n)
	}
	if name := s.readString(t, "tag", "name", applied.TagID); name != word {
		t.Fatalf("case 2 criterion 6: the applied tag reads %q, want %q", name, word)
	}
}

// The other half of criterion 6, and the reason the vocabulary is governed: a
// word the workspace has never chosen is REFUSED, not coined. An assistant
// that could mint one would put a misspelling of somebody's tag into the
// shared vocabulary permanently, which is the drift a shared vocabulary is for
// preventing.
func TestCase2AnUnknownTagNameIsRefusedRatherThanCoined(t *testing.T) {
	s := boot(t, scopesReadWrite)
	person := s.createFromCard(t, cardFields(cardEmail))

	const unknown = "Tech Sauce Bangkok 2027"
	s.MCP.CallRefused(t, "apply_tag", map[string]any{
		"tag_name": unknown, "record_type": "person", "record_id": person.ID.String(),
	})

	// Refused AND nothing written: a call that coined the word and then failed
	// for some other reason would leave the same permanent row behind.
	if n := s.countRows(t, `SELECT count(*) FROM tag WHERE name = $1`, unknown); n != 0 {
		t.Fatalf("the refused name exists as a tag (%d row(s)); apply_tag coined a word only an admin may add", n)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// The brief without a model.
//
// Every sentence cites the record it came from, exactly as a model-written one
// would, so the card renders and behaves identically whichever wrote it. That
// is the point of the floor: a workspace with no model lane gets a plainer
// brief, not a blank card where the brief should be, and `generated_by` says
// which it is rather than passing one off as the other.

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/personcontext"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The citable record types. A brief may only point at things the reader can
// open, and these are the ones the person page can open in place.
//
// DERIVED from the contract's own enum rather than re-spelled, because the wire
// converts a citation's type straight to that enum: a hand-typed copy would let
// a rename upstream leave the grounding filter matching a type the wire no
// longer carries — a citation that silently stops grounding, on a card whose
// whole promise is that a reader can check it.
const (
	citePerson   = string(crmcontracts.OrganizationBriefEvidenceEntityTypePerson)
	citeActivity = string(crmcontracts.OrganizationBriefEvidenceEntityTypeActivity)
	citeDeal     = string(crmcontracts.OrganizationBriefEvidenceEntityTypeDeal)
)

// Deterministic writes the brief from the assembled input alone.
//
// The order answers the questions a reader asks in the order they ask them:
// who is this person commercially, what is due about them now, what have they
// said they care about, and what was last said.
func Deterministic(personID string, in Input) []Sentence {
	self := []Evidence{{EntityType: citePerson, EntityID: personID}}
	sentences := make([]Sentence, 0, maxSentences)

	sentences = append(sentences, Sentence{Text: identityLine(in), Evidence: self})

	if line, evidence, ok := dueNowLine(in, self); ok {
		sentences = append(sentences, Sentence{Text: line, Evidence: evidence})
	}
	if in.OpenDeal != nil {
		sentences = append(sentences, Sentence{
			Text:     dealLine(in),
			Evidence: []Evidence{{EntityType: citeDeal, EntityID: in.OpenDeal.ID}},
		})
	}
	if line, evidence, ok := caresAboutLine(in); ok {
		sentences = append(sentences, Sentence{Text: line, Evidence: evidence})
	}
	if len(in.Recent) > 0 {
		last := in.Recent[0]
		sentences = append(sentences, Sentence{
			Text:     lastTouchLine(in, last),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: last.ID}},
		})
	}
	return claims.Dedupe(sentences)
}

// dueNowLine states what the page's own ladder decided is due about this
// contact, or — with no moment — what most recently moved.
//
// The moment's headline is carried VERBATIM. It is already written from the
// evidence by the ladder that selected it, and a floor that reworded it would
// be a second spelling of the same finding, free to disagree with the one the
// page prints beside this card.
func dueNowLine(in Input, self []Evidence) (string, []Evidence, bool) {
	if in.Moment != nil && in.Moment.Headline != "" {
		return claims.TerminateSentence(in.Moment.Headline), momentEvidence(in, self), true
	}
	if len(in.Changes) == 0 {
		return "", nil, false
	}
	return changeLine(in.Changes[0]), self, true
}

// momentEvidence cites the records the moment fired on, and the person when it
// fired on derived facts alone. A sentence citing nothing is dropped whole, so
// the fallback is what keeps a moment with no rows behind it on the card.
func momentEvidence(in Input, self []Evidence) []Evidence {
	if len(in.Moment.Sources) == 0 {
		return self
	}
	evidence := make([]Evidence, 0, len(in.Moment.Sources))
	for _, source := range in.Moment.Sources {
		evidence = append(evidence, Evidence{EntityType: citeActivity, EntityID: source})
	}
	return evidence
}

// changeLine says what moved. The span and the bands come from the record; a
// kind this build does not know renders as the stored key rather than as an
// invented sentence about it.
func changeLine(change ChangeIn) string {
	switch change.Kind {
	case string(crmcontracts.RepliedAfterGap):
		if change.Days > 0 {
			return fmt.Sprintf("They answered after %d days of silence.", change.Days)
		}
		return "They answered after a long silence."
	case string(crmcontracts.WentQuiet):
		if change.Days > 0 {
			return fmt.Sprintf("This relationship has been quiet for %d days.", change.Days)
		}
		return "This relationship has gone quiet."
	case string(crmcontracts.Warmed), string(crmcontracts.Cooled):
		return fmt.Sprintf("The relationship moved from %s to %s.",
			readableBand(change.From), readableBand(change.To))
	default:
		return fmt.Sprintf("The relationship changed: %s.", readableRole(change.Kind))
	}
}

// readableBand names a strength band, falling back to the stored key for a band
// this build does not know — the same rule readableRole follows, for the same
// reason: inventing a label for a value nobody defined would be a claim.
func readableBand(band string) string {
	if band == "" {
		return "unrecorded"
	}
	return readableRole(band)
}

// identityLine says who this person is in the current commercial context —
// the first thing a reader needs and the one sentence that is always true.
func identityLine(in Input) string {
	switch {
	case in.Title != "" && in.Employer != "":
		return fmt.Sprintf("%s is %s at %s.", in.Name, in.Title, in.Employer)
	case in.Employer != "":
		return fmt.Sprintf("%s works at %s.", in.Name, in.Employer)
	case in.Title != "":
		return fmt.Sprintf("%s is %s.", in.Name, in.Title)
	default:
		return fmt.Sprintf("%s is recorded here with no title or employer.", in.Name)
	}
}

// dealLine states the commercial stake, with the seat this person holds on it.
// The role is stored relationship data — it is never inferred from a title.
func dealLine(in Input) string {
	deal := in.OpenDeal
	parts := []string{deal.Name}
	if spoken := personcontext.SpokenAmount(deal.AmountMinor, deal.Currency); spoken != "" {
		parts = append(parts, spoken)
	}
	if deal.Stage != "" {
		parts = append(parts, deal.Stage)
	}
	line := strings.Join(parts, " · ")
	if in.BuyingRole != "" {
		return fmt.Sprintf("They are the recorded %s on %s.", readableRole(in.BuyingRole), line)
	}
	return fmt.Sprintf("They sit on %s, with no buying role recorded.", line)
}

// readableRole turns the stored role key into words. The keys are a naming
// convention rather than an enum, so an unrecognized one is rendered as it was
// stored — inventing a label for a role nobody defined would be a claim.
func readableRole(role string) string {
	return strings.ReplaceAll(role, "_", " ")
}

// caresAboutLine names what this person has explicitly said matters, citing the
// conversation it was said in rather than the derived claim row — the reader
// checks a sentence against what was actually written.
func caresAboutLine(in Input) (string, []Evidence, bool) {
	priorities := claimsOfKind(in, string(crmcontracts.Priority))
	objections := claimsOfKind(in, string(crmcontracts.Objection))
	if len(priorities) == 0 && len(objections) == 0 {
		return "", nil, false
	}
	var parts []string
	var evidence []Evidence
	if len(priorities) > 0 {
		parts = append(parts, fmt.Sprintf("focused on %s", priorities[0].Body))
		evidence = append(evidence, Evidence{EntityType: citeActivity, EntityID: priorities[0].SourceID})
	}
	if len(objections) > 0 {
		parts = append(parts, fmt.Sprintf("with %s still unresolved", objections[0].Body))
		evidence = append(evidence, Evidence{EntityType: citeActivity, EntityID: objections[0].SourceID})
	}
	return fmt.Sprintf("They are %s.", strings.Join(parts, ", ")), evidence, true
}

func claimsOfKind(in Input, kind string) []ClaimIn {
	var out []ClaimIn
	for _, claim := range in.Claims {
		if claim.Kind == kind {
			out = append(out, claim)
		}
	}
	return out
}

// lastTouchLine says which direction went last and what was actually said,
// because those are the whole question: a contact we wrote to a fortnight ago
// with no reply and one who wrote to us this morning have the same last-touch
// date and opposite meanings, and neither is worth a sentence if the sentence
// cannot say what the message was about.
func lastTouchLine(in Input, last ActIn) string {
	switch {
	case last.Withheld:
		// The date is the reader's even though the words are not, and saying so
		// is the honest sentence: silence here reads as nobody having written.
		return "The most recent message on this contact is one you may not read."
	case in.LastInbound != "" && in.LastInbound > in.LastOutbound:
		return fmt.Sprintf("They wrote last, %s, and it is unanswered.", aboutClause(last))
	case in.LastOutbound != "":
		return fmt.Sprintf("You wrote last, %s, with no reply yet.", aboutClause(last))
	default:
		return fmt.Sprintf("The last thing captured was %s.", aboutClause(last))
	}
}

// aboutClause names what a message was about, preferring the sender's own line
// to the subject. A row that carries neither is named by its kind, which says
// only that something happened — the honest reading of a row that recorded
// nothing else.
func aboutClause(last ActIn) string {
	if last.Preview != "" {
		return fmt.Sprintf("saying %q", trimmedPreview(last.Preview))
	}
	if last.Subject != "" {
		return fmt.Sprintf("about %q", last.Subject)
	}
	return "a " + readableRole(last.Kind)
}

// previewWords bounds the quoted line. A preview is one line by construction,
// but "one line" is the projection's promise about newlines and not about
// length, and a card is four sentences wide.
const previewWords = 18

// trimmedPreview cuts a long preview at a word boundary and marks the cut, so a
// reader can tell a quotation that ended from one that was shortened.
func trimmedPreview(preview string) string {
	words := strings.Fields(preview)
	if len(words) <= previewWords {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:previewWords], " ") + "…"
}

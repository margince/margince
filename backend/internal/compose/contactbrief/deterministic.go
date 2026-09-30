// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

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
	"github.com/margince/margince/backend/internal/compose/contactcontext"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The citable record types. A brief may only point at things the reader can
// open, and these are the ones the contact page can open in place.
//
// DERIVED from the contract's own enum rather than re-spelled, because the wire
// converts a citation's type straight to that enum: a hand-typed copy would let
// a rename upstream leave the grounding filter matching a type the wire no
// longer carries — a citation that silently stops grounding, on a card whose
// whole promise is that a reader can check it.
const (
	citeContact  = string(crmcontracts.CompanyBriefEvidenceEntityTypeContact)
	citeActivity = string(crmcontracts.CompanyBriefEvidenceEntityTypeActivity)
	citeDeal     = string(crmcontracts.CompanyBriefEvidenceEntityTypeDeal)
)

// Deterministic writes the brief from the assembled input alone.
//
// The order answers the questions a reader asks in the order they ask them:
// who is this contact commercially, what is due about them now, what have they
// said they care about, and what was last said.
func Deterministic(contactID string, in Input, lang string) []Sentence {
	say := phrasesFor(lang)
	self := []Evidence{{EntityType: citeContact, EntityID: contactID}}
	sentences := make([]Sentence, 0, maxSentences)

	sentences = append(sentences, Sentence{Text: identityLine(in, say), Evidence: self})

	if line, evidence, ok := dueNowLine(in, self, say); ok {
		sentences = append(sentences, Sentence{Text: line, Evidence: evidence})
	}
	if in.OpenDeal != nil {
		sentences = append(sentences, Sentence{
			Text:     dealLine(in, say),
			Evidence: []Evidence{{EntityType: citeDeal, EntityID: in.OpenDeal.ID}},
		})
	}
	if line, evidence, ok := caresAboutLine(in, say); ok {
		sentences = append(sentences, Sentence{Text: line, Evidence: evidence})
	}
	if len(in.Recent) > 0 {
		last := in.Recent[0]
		sentences = append(sentences, Sentence{
			Text:     lastTouchLine(in, last, say),
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
func dueNowLine(in Input, self []Evidence, say spoken) (string, []Evidence, bool) {
	if in.Moment != nil && in.Moment.Headline != "" {
		return claims.TerminateSentence(in.Moment.Headline), momentEvidence(in, self), true
	}
	if len(in.Changes) == 0 {
		return "", nil, false
	}
	return changeLine(in.Changes[0], say), self, true
}

// momentEvidence cites the records the moment fired on, and the contact when it
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
func changeLine(change ChangeIn, say spoken) string {
	switch change.Kind {
	case string(crmcontracts.ContactRelationshipChangeKindRepliedAfterGap):
		switch {
		case change.Days == 1:
			return say.say(floor.AnsweredAfterADay)
		case change.Days > 1:
			return fmt.Sprintf(say.say(floor.AnsweredAfterDays), change.Days)
		}
		return say.say(floor.AnsweredAfterLong)
	case string(crmcontracts.ContactRelationshipChangeKindWentQuiet):
		switch {
		case change.Days == 1:
			return say.say(floor.QuietForADay)
		case change.Days > 1:
			return fmt.Sprintf(say.say(floor.QuietForDays), change.Days)
		}
		return say.say(floor.GoneQuiet)
	case string(crmcontracts.ContactRelationshipChangeKindWarmed), string(crmcontracts.ContactRelationshipChangeKindCooled):
		return fmt.Sprintf(say.say(floor.BandMoved),
			readableBand(change.From, say), readableBand(change.To, say))
	default:
		return fmt.Sprintf(say.say(floor.RelationshipMoved), readableRole(change.Kind))
	}
}

// readableBand names a strength band, falling back to the stored key for a band
// this build does not know — the same rule readableRole follows, for the same
// reason: inventing a label for a value nobody defined would be a claim.
func readableBand(band string, say spoken) string {
	if band == "" {
		return say.say(floor.UnrecordedBand)
	}
	return readableRole(band)
}

// identityLine says who this contact is in the current commercial context —
// the first thing a reader needs and the one sentence that is always true.
func identityLine(in Input, say spoken) string {
	switch {
	case in.Title != "" && in.Employer != "":
		return fmt.Sprintf(say.say(floor.IdentityTitleEmployer), in.Name, in.Title, in.Employer)
	case in.Employer != "":
		return fmt.Sprintf(say.say(floor.IdentityEmployer), in.Name, in.Employer)
	case in.Title != "":
		return fmt.Sprintf(say.say(floor.IdentityTitle), in.Name, in.Title)
	default:
		return fmt.Sprintf(say.say(floor.IdentityBare), in.Name)
	}
}

// dealLine states the commercial stake, with the seat this contact holds on it.
// The role is stored relationship data — it is never inferred from a title.
func dealLine(in Input, say spoken) string {
	deal := in.OpenDeal
	parts := []string{deal.Name}
	if spoken := contactcontext.SpokenAmount(deal.AmountMinor, deal.Currency); spoken != "" {
		parts = append(parts, spoken)
	}
	if deal.Stage != "" {
		parts = append(parts, deal.Stage)
	}
	line := strings.Join(parts, " · ")
	if in.BuyingRole != "" {
		return fmt.Sprintf(say.say(floor.RecordedRoleOnDeal), readableRole(in.BuyingRole), line)
	}
	return fmt.Sprintf(say.say(floor.OnDealNoRole), line)
}

// readableRole turns the stored role key into words. The keys are a naming
// convention rather than an enum, so an unrecognized one is rendered as it was
// stored — inventing a label for a role nobody defined would be a claim.
func readableRole(role string) string {
	return strings.ReplaceAll(role, "_", " ")
}

// caresAboutLine names what this contact has explicitly said matters, citing the
// conversation it was said in rather than the derived claim row — the reader
// checks a sentence against what was actually written.
func caresAboutLine(in Input, say spoken) (string, []Evidence, bool) {
	priorities := claimsOfKind(in, string(crmcontracts.ConversationClaimKindPriority))
	objections := claimsOfKind(in, string(crmcontracts.ConversationClaimKindObjection))
	switch {
	case len(priorities) > 0 && len(objections) > 0:
		return fmt.Sprintf(say.say(floor.CaresBoth), priorities[0].Body, objections[0].Body), []Evidence{
			{EntityType: citeActivity, EntityID: priorities[0].SourceID},
			{EntityType: citeActivity, EntityID: objections[0].SourceID},
		}, true
	case len(priorities) > 0:
		return fmt.Sprintf(say.say(floor.CaresPriority), priorities[0].Body),
			[]Evidence{{EntityType: citeActivity, EntityID: priorities[0].SourceID}}, true
	case len(objections) > 0:
		return fmt.Sprintf(say.say(floor.CaresObjection), objections[0].Body),
			[]Evidence{{EntityType: citeActivity, EntityID: objections[0].SourceID}}, true
	default:
		return "", nil, false
	}
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
func lastTouchLine(in Input, last ActIn, say spoken) string {
	switch {
	case last.Withheld:
		// The date is the reader's even though the words are not, and saying so
		// is the honest sentence: silence here reads as nobody having written.
		return say.say(floor.WithheldMessage)
	case in.LastInbound != "" && in.LastInbound > in.LastOutbound:
		if outstanding(last) {
			return fmt.Sprintf(say.say(floor.TheyWroteLastOpen), aboutClause(last, say))
		}
		return fmt.Sprintf(say.say(floor.TheyWroteLastClosed), aboutClause(last, say))
	case in.LastOutbound != "":
		if outstanding(last) {
			return fmt.Sprintf(say.say(floor.YouWroteLastOpen), aboutClause(last, say))
		}
		return fmt.Sprintf(say.say(floor.YouWroteLastClosed), aboutClause(last, say))
	default:
		return fmt.Sprintf(say.say(floor.LastCaptured), aboutClause(last, say))
	}
}

// The two moves that mean somebody still owes something. DERIVED from the
// contract's own enum rather than re-spelled, the way SpeakerFor derives the
// speakers beside it: a rename upstream fails to compile here instead of
// silently making every message read as settled.
const (
	moveNeedsReply     = string(crmcontracts.EmailSummaryMoveNeedsReply)
	moveWaitingForThem = string(crmcontracts.EmailSummaryMoveWaitingForThem)
)

// answerStateClause decides whether the floor may say a message is outstanding.
//
// The DIRECTION alone cannot answer it, which is what this exists to stop the
// floor claiming. "They wrote last" is true of a scheduling note that settled
// the thing it was about, and the floor reported exactly that as unanswered on
// a contact whose meeting was agreed — the words said settled and the frame
// said owed, in one sentence.
//
// Move is the server's own reading of whose turn it is, and it is EMPTY where
// the question cannot be answered honestly: on a row that is not mail, on one
// with no summary, and on `none` — which the fold leaves empty rather than
// spelling out. Empty therefore says nothing rather than guessing, and the
// sentence simply ends after what the message was about.
func outstanding(last ActIn) bool {
	return last.Move == moveNeedsReply || last.Move == moveWaitingForThem
}

// aboutClause names what a message was about, preferring the sender's own line
// to the subject. A row that carries neither is named by its kind, which says
// only that something happened — the honest reading of a row that recorded
// nothing else.
func aboutClause(last ActIn, say spoken) string {
	if last.Preview != "" {
		return fmt.Sprintf(say.say(floor.AboutSaying), trimmedPreview(last.Preview))
	}
	if last.Subject != "" {
		return fmt.Sprintf(say.say(floor.AboutSubject), last.Subject)
	}
	return fmt.Sprintf(say.say(floor.AboutKind), readableRole(last.Kind))
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

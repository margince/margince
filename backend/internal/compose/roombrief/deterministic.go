// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// The citable record types, derived from the contract's enum so a rename
// upstream fails to compile rather than silently stops grounding.
const (
	citeParticipant = string(crmcontracts.DealRoomBriefEvidenceEntityTypeParticipant)
	citeDocument    = string(crmcontracts.DealRoomBriefEvidenceEntityTypeDocument)
	citeThread      = string(crmcontracts.DealRoomBriefEvidenceEntityTypeThread)
)

// maxSentences bounds the brief; it sits above a tab already full of the
// records it reads.
const maxSentences = 4

// waitingQuoted is how many waiting threads the floor quotes before it only
// counts the rest.
const waitingQuoted = 2

// quoteRunes bounds a quoted comment, so one long message cannot be the brief.
const quoteRunes = 140

// Deterministic writes the brief from the input alone. It answers in the order
// a seller asks: who is waiting on me, who cannot get in, who has not come.
func Deterministic(in Input, lang string) []Sentence {
	say := langcopy.For(lang)
	var out []Sentence
	var waiting []ThreadIn
	for _, thread := range in.Threads {
		if thread.WaitingOnYou {
			waiting = append(waiting, thread)
		}
	}
	for _, thread := range waiting[:min(len(waiting), waitingQuoted)] {
		out = append(out, waitingLine(thread, say))
	}
	if rest := waiting[min(len(waiting), waitingQuoted):]; len(rest) > 0 {
		out = append(out, moreWaitingLine(rest, say))
	}
	var notSignedIn []BuyerIn
	for _, buyer := range in.Buyers {
		if buyer.AskedForNewLinkOn != "" {
			out = append(out, Sentence{
				Text:     fmt.Sprintf(say.Say(floor.AskedForLink), buyer.Name),
				Nature:   natureFact,
				Evidence: []Evidence{{EntityType: citeParticipant, EntityID: buyer.ID}},
			})
			continue
		}
		if !buyer.SignedIn {
			notSignedIn = append(notSignedIn, buyer)
		}
	}
	if len(notSignedIn) > 0 {
		out = append(out, notSignedInLine(notSignedIn, say))
	}
	if len(out) == 0 && len(in.Threads) > 0 {
		out = append(out, nothingOwedLine(in.Threads, say))
	}
	return out[:min(len(out), maxSentences)]
}

func waitingLine(thread ThreadIn, say langcopy.Spoken) Sentence {
	last := thread.Comments[len(thread.Comments)-1]
	quote := strings.Join(strings.Fields(textcut.Runes(last.Body, quoteRunes)), " ")
	if len([]rune(last.Body)) > quoteRunes {
		quote += "…"
	}
	evidence := []Evidence{{EntityType: citeThread, EntityID: thread.ID}}
	text := fmt.Sprintf(say.Say(floor.WaitingRoom), last.Author, quote)
	if thread.About != "" {
		text = fmt.Sprintf(say.Say(floor.WaitingAbout), last.Author, thread.About, quote)
		evidence = append(evidence, Evidence{EntityType: citeDocument, EntityID: thread.DocumentID})
	}
	return Sentence{Text: text, Nature: natureFact, Evidence: evidence}
}

func moreWaitingLine(rest []ThreadIn, say langcopy.Spoken) Sentence {
	evidence := make([]Evidence, 0, len(rest))
	for _, thread := range rest {
		evidence = append(evidence, Evidence{EntityType: citeThread, EntityID: thread.ID})
	}
	text := say.Say(floor.OneMore)
	if len(rest) > 1 {
		text = fmt.Sprintf(say.Say(floor.MoreWaiting), strconv.Itoa(len(rest)))
	}
	return Sentence{Text: text, Nature: natureFact, Evidence: evidence}
}

func notSignedInLine(buyers []BuyerIn, say langcopy.Spoken) Sentence {
	names := make([]string, 0, len(buyers))
	evidence := make([]Evidence, 0, len(buyers))
	for _, buyer := range buyers {
		names = append(names, buyer.Name)
		evidence = append(evidence, Evidence{EntityType: citeParticipant, EntityID: buyer.ID})
	}
	return Sentence{
		Text:     fmt.Sprintf(say.Say(floor.NotSignedIn), strings.Join(names, ", ")),
		Nature:   natureFact,
		Evidence: evidence,
	}
}

// nothingOwedLine is said only of a room with a conversation. Of an empty room
// it would be true and useless, so an empty room has no brief.
func nothingOwedLine(threads []ThreadIn, say langcopy.Spoken) Sentence {
	evidence := make([]Evidence, 0, len(threads))
	for _, thread := range threads {
		evidence = append(evidence, Evidence{EntityType: citeThread, EntityID: thread.ID})
	}
	return Sentence{Text: say.Say(floor.NothingOwed), Nature: natureFact, Evidence: evidence}
}

// Both writers speak the one claim vocabulary, so the grounding rule proved for
// the other briefs holds here.
type (
	// Sentence is one written claim with the records it was written from.
	Sentence = claims.Sentence
	// Evidence is one record a sentence cites.
	Evidence = claims.Evidence
)

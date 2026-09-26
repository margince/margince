// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

// The claims no band makes true.
//
// Every other list in this package is gated on the conversation's state,
// because what it refuses is a claim about the recipient's MEMORY: "as
// discussed" is ordinary in a live exchange and false after eight months. These
// two are not gated, because what they refuse is a claim about the WORLD. A
// call either happened or it did not, and a sentence was either written or it
// was not — no amount of recent correspondence makes an invented one true.

import (
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Grounds is what the RECORD supports, as opposed to what the draft says.
//
// A struct rather than two bools side by side: they are both about "can this
// claim be sourced", they were adjacent in the signature, and a caller that
// transposed them would ground a first message on a thread it does not have
// and refuse a booked meeting its own day — silently, and in both directions.
type Grounds struct {
	// Threaded: this draft answers a real inbound message, so it may echo a
	// call the counterparty themselves named.
	Threaded bool
	// Booked: the record carries a meeting with this recipient, so the draft
	// may refer to its day.
	Booked bool
	// Met: the caller's own intent names an earlier meeting with this
	// recipient, so the draft may say they met. Set it from IntentNamesMeeting.
	Met bool
}

// IntentNamesMeeting reports whether the caller's intent says an encounter
// already HAPPENED. The caller can know that; nothing else in the input can.
//
// Every language at once, because the intent is typed in whatever the rep
// writes in, not the draft's language. A clause counts when it carries an
// encounter frame and no negation: the frame keeps "propose a meeting" and the
// idiom "eine Entscheidung getroffen" out, the negation check keeps "we have
// not met yet" out, and either one read as met grounds the invention the rule
// refuses.
func IntentNamesMeeting(intent string) bool {
	for _, clause := range clauses(strings.ToLower(intent)) {
		if negated(clause) {
			continue
		}
		for _, frames := range encounterFrame {
			for _, frame := range frames {
				if inOrder(clause, frame) {
					return true
				}
			}
		}
	}
	return false
}

// encounterFrame are the ways an intent says the rep and the recipient have
// met. A frame is words that must appear in that order within one clause, so
// German can say "wir haben uns auf der Messe getroffen" with the fair between
// the reflexive and the participle — the reflexive is what makes it a meeting.
var encounterFrame = map[textlang.Lang][][]string{
	textlang.English: {
		{"we met"},
		{"we've met"},
		{"we have met"},
		{"we had met"},
		{"met you"},
		{"met them"},
		{"met him"},
		{"met her"},
		{"after meeting"},
		{"since meeting"},
		{"having met"},
		{"we spoke"},
		{"we talked"},
		{"after speaking"},
		{"after talking"},
		{"after the call"},
		{"after our call"},
		{"after the meeting"},
		{"after our meeting"},
		{"following our call"},
		{"following our meeting"},
	},
	textlang.German: {
		{"kennengelernt"},
		{"uns", "getroffen"},
		{"uns", "begegnet"},
		{"wir", "gesprochen"},
		{"nach dem treffen"},
		{"nach unserem treffen"},
		{"nach dem gespräch"},
		{"nach unserem gespräch"},
		{"nach dem telefonat"},
		{"nach unserem telefonat"},
	},
	textlang.Vietnamese: {
		{"đã gặp"}, {"sau buổi gặp"}, {"đã trao đổi"},
	},
}

// negation are the words that turn an encounter frame into its denial.
var negation = []string{
	"not", "never", "nicht", "nie", "niemals", "kein", "keine", "noch nie", "chưa", "không",
}

// clauseJoiners split one sentence into the clauses a negation binds to, so
// "we met at the fair and have not spoken since" still names the meeting.
var clauseJoiners = []string{"and", "but", "und", "aber", "sondern", "và", "nhưng"}

// clauses is lowered text cut at sentence ends and at clause joiners.
func clauses(lowered string) []string {
	for _, joiner := range clauseJoiners {
		lowered = strings.ReplaceAll(lowered, " "+joiner+" ", ";")
	}
	return strings.FieldsFunc(lowered, func(r rune) bool { return strings.ContainsRune(".;!?\n", r) })
}

// negated reports a clause that denies what it says. "n't" is matched inside
// the word, where "haven't" carries it.
func negated(clause string) bool {
	if strings.Contains(clause, "n't") || strings.Contains(clause, "n\u2019t") {
		return true
	}
	for _, word := range negation {
		if contains(clause, word) {
			return true
		}
	}
	return false
}

// inOrder reports whether clause holds every part of frame as whole words, in
// order.
func inOrder(clause string, frame []string) bool {
	rest := clause
	for _, part := range frame {
		i := wordIndex(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
	}
	return true
}

// inventedConversation is the claim that a conversation took place, where
// nothing grounds one. An intent naming a meeting grounds the ENCOUNTER only,
// so "it was a pleasure meeting you" stands and "after our call" is refused.
//
// The grounded phrases are cut out of the text rather than skipped in the
// list, because the two overlap: "freute mich, sie kennenzulernen" carries
// "freute mich", which on its own claims a conversation.
func inventedConversation(lowered string, lang textlang.Lang, met bool) []Finding {
	phrases := append(slices.Clone(metEncounter[lang]), spokenExchange[lang]...)
	if met {
		for _, grounded := range metEncounter[lang] {
			lowered = strings.ReplaceAll(lowered, grounded, " ")
		}
		phrases = spokenExchange[lang]
	}
	return firstMatch(lowered, phrases, RuleInventedConversation,
		"this message opens a new conversation, so nothing in the input says a "+
			"call or meeting took place — write from the messages on the record")
}

// metEncounter are the ways a draft says the sender and the recipient MET — the
// one conversation claim an intent naming the meeting makes true. The bar is
// spokenExchange's: the past tense is in the phrase.
var metEncounter = map[textlang.Lang][]string{
	textlang.English: {"was a pleasure meeting"},
	textlang.German: {
		"hat mich gefreut, sie kennenzulernen", "hat mich gefreut, dich kennenzulernen",
		"freute mich, sie kennenzulernen", "freute mich, dich kennenzulernen",
	},
}

// spokenExchange are the ways a draft asserts that a CONVERSATION happened —
// a call, a meeting, a chat — as opposed to the messages the record holds.
//
// Checked at every band, which is what separates this list from assumedMemory
// above. "As discussed" at band fresh is ordinary: an exchange is running and
// both sides are holding it. "It was a pleasure connecting earlier this week"
// is a different claim entirely — it says two contacts SPOKE, on a date, and
// nothing in any drafting input can support that. The 360 carries activities;
// a meeting the drafter can see is one on the calendar, and a calendar entry is
// not evidence that it took place or that anyone enjoyed it.
//
// The reported defect that produced this list: a company with a live support
// thread and no call whatsoever was drafted an opener reading "Marine, it was a
// pleasure connecting earlier this week." Every band's list was checked against
// that text and every one of them passed it.
// Every entry asserts a COMPLETED exchange on its own, which is the bar an
// entry has to clear. "Great to connect" does not: "it would be great to
// connect next week" is an ask, and the same words a sentence later are a
// memory. "We can cover that in our call tomorrow" is a plan. A list that
// refuses those refuses the drafts this product exists to write, so the past
// tense has to be IN the phrase rather than assumed around it.
var spokenExchange = map[textlang.Lang][]string{
	textlang.English: {
		"it was a pleasure connecting", "was a pleasure speaking",
		"was a pleasure talking",
		"was good speaking", "was great speaking", "was good talking",
		"was great talking", "was good to connect", "was great to connect",
		"was nice to connect", "was nice speaking",
		"after our call", "after our conversation", "during our call",
		"on the call last", "our conversation earlier", "when we spoke",
		"thanks for taking the time to speak",
	},
	textlang.German: {
		"freute mich",
		"nach unserem gespräch", "nach unserem telefonat", "nach unserem call",
		"in unserem gespräch letzte", "danke für das gespräch",
	},
	textlang.Vietnamese: {
		"rất vui được trao đổi với", "sau cuộc gọi", "sau buổi trao đổi",
	},
}

// attributedClaim are the ways a draft puts words in the recipient's mouth.
//
// Also every band, and for the same reason. A drafting surface knows what its
// input said, never who said it: an activity reaches a contact or an account
// through links that record what a message CONCERNS rather than who wrote it,
// and no 360 carries participants. The contact and account prompts both state
// this ("say 'the question about X' and never 'you wrote'"), which is exactly
// why it needs a check — a rule the prompt states is a rule the model has
// already been observed breaking three times in this program.
//
// The stems rather than whole phrases. Enumerating completions is how the
// earlier lists lost: "introduction by" missed "introduction to", and a German
// list of wellbeing completions missed the one the model actually wrote.
var attributedClaim = map[textlang.Lang][]string{
	textlang.English: {
		"you mentioned", "you said", "you raised", "you told me", "you noted",
		"you indicated", "you expressed", "you described", "you flagged",
		"you brought up", "you pointed out", "as you put it",
	},
	textlang.German: {
		"sie erwähnten", "du erwähntest", "sie sagten", "du sagtest",
		"sie erwähnt haben", "du erwähnt hast", "sie angesprochen haben",
		"du angesprochen hast", "wie sie sagten", "wie du sagtest",
	},
	textlang.Vietnamese: {
		"anh/chị đã đề cập", "anh/chị đã nói",
	},
}

// scheduledArrangement are the ways a draft asserts a meeting is ALREADY SET,
// on a day, when the record books none.
//
// Checked only where nothing is booked, which is what separates it from
// spokenExchange above. With a meeting on file "am Donnerstag" is the drafter
// doing its job — the contact prompt asks for exactly that phrasing over a
// timestamp. With none, the same words hand a customer an appointment nobody
// made.
//
// The reported defect: a request for two untranslated product examples, with no
// meeting anywhere in the input, produced "unsere geplante Demonstration der
// Übersetzungsregeln für morgen".
//
// THE PHRASE HAS TO ASSERT THE BOOKING, not merely name a day. A draft that
// PROPOSES one is what this product exists to write — "it would be great to
// connect next week", "es freut mich, dass wir nächste Woche sprechen können" —
// and a list refusing those refuses the good drafts along with the invented
// ones. So the entries carry the possessive or the planning word that makes the
// arrangement a settled fact: "our call tomorrow" asserts a call exists,
// "shall we speak tomorrow" asks for one. That distinction is why this is a
// phrase list and not a list of days.
var scheduledArrangement = map[textlang.Lang][]string{
	textlang.English: {
		"our meeting tomorrow", "our call tomorrow", "our demo tomorrow",
		"our meeting next week", "our call next week",
		"the meeting tomorrow", "the call tomorrow", "the demo tomorrow",
		"scheduled for tomorrow", "planned for tomorrow", "booked for tomorrow",
		"as scheduled tomorrow",
	},
	// German pays for "morgen" twice: it is both "tomorrow" and "morning", so
	// the bare word appears in "guten Morgen" and "morgen früh" without
	// scheduling anything. Every entry here carries the arrangement with it.
	textlang.German: {
		"geplante demonstration", "geplanten demonstration", "geplante vorführung",
		"unser termin morgen", "unser gespräch morgen", "unser telefonat morgen",
		"unsere demo morgen", "für morgen geplant", "für morgen vereinbart",
		"für morgen angesetzt", "termin für morgen", "demonstration für morgen",
	},
	textlang.Vietnamese: {
		"cuộc họp ngày mai", "buổi demo ngày mai", "đã lên lịch ngày mai",
	},
}

// directedIntroduction are the ways a draft says who introduced, referred or
// put in touch whom — the RELATIONSHIPS rule every drafting prompt states.
//
// Every band and every thread, because no record makes one true: the product
// holds no contact-to-contact referral, so the fact was read out of quoted text,
// where "Dass Romina den Kontakt hergestellt hat" came back with the direction
// reversed. Directed forms only; "I would like to introduce myself" is the
// sender's own act, which the prompt asks for.
var directedIntroduction = map[textlang.Lang][]string{
	textlang.English: {
		"introduced us", "introduced me to", "introduced you to", "introduced by",
		"introduction by", "introduction from", "introduction via",
		"put us in touch", "put me in touch", "connected us",
		"referred us", "referred me", "referred you", "referred by", "referral from",
	},
	textlang.German: {
		"kontakt hergestellt", "in kontakt gebracht", "kontakt vermittelt",
		"uns vermittelt", "vorgestellt von", "vorgestellt durch", "vorstellung durch",
		"empfohlen von", "empfehlung von", "empfehlung durch",
	},
	textlang.Vietnamese: {
		"được giới thiệu bởi", "giới thiệu bởi",
	},
}

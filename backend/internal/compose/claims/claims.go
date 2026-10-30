// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package claims is the one grounding filter and the one claim vocabulary the
// generated surfaces share — the account brief, the company dossier and the
// growth-fit assessment (DOSS-FORM-1).
//
// It is one implementation on purpose. Three copies of a grounding rule is
// three chances for one of them to be lenient, and a lenient one renders a
// sentence nobody can check as though it had been checked. The rule is the
// product's whole answer to "how do I know this is true", so it has exactly one
// spelling.
//
// What lives here is the part that does not depend on WHICH surface is asking:
// the claim natures, the citation shape, and the decision to keep or drop a
// sentence given the records the assembler actually supplied. Building that
// supplied set is each surface's own job — the brief knows about deals and
// tasks, the dossier about profile fields and facts — and neither knows about
// the other's inputs.
package claims

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/kernel/specifics"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Evidence points at one record a sentence rests on. It names a record the
// READER can already open: every surface here assembles under the reader's own
// row scope, so a citation can never name a row they would be refused.
type Evidence struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	// Name is the record's own display name, when the caller had it at hand —
	// a deal's name, an activity's subject. Descriptive only: it plays no part
	// in identity, so Grounded compares type and id alone (a citation the
	// writer forgot to name must still ground on the pair it actually has).
	Name string `json:"name,omitempty"`
}

// identity is the pair Grounded and Dedupe key on: the record a citation
// names, never the name it happens to carry alongside that. Comparing full
// Evidence values would make a citation's grounding depend on whether SOME
// caller remembered to set Name — the known-set built at knownRecords time
// never does, so a name-bearing sentence would fail every lookup against it.
func identity(e Evidence) Evidence {
	return Evidence{EntityType: e.EntityType, EntityID: e.EntityID}
}

// Sentence is one claim plus the records it was written from.
type Sentence struct {
	Text string `json:"text"`
	// What KIND of claim this is (DOSS-PARAM-1). Empty means fact, which is the
	// only permitted compatibility default — a reader forgives a wrong opinion
	// and does not forgive a wrong fact, so the two that are NOT fact are the
	// ones that must be said out loud.
	Nature   string     `json:"nature,omitempty"`
	Evidence []Evidence `json:"evidence"`
}

// idInProse matches a record id written into a sentence, spelled ONCE for every
// surface and the tests that gate them. It is the UUID shape every citable
// record carries, so a reply that pastes one anywhere in its prose is caught
// wherever in the clause it landed.
var idInProse = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// SpellsRecordID reports whether prose hands the reader a raw record id. It is
// exported because it is a claim a surface's own tests assert directly — every
// deterministic answer has to be readable prose, whichever writer produced it.
func SpellsRecordID(text string) bool {
	return idInProse.MatchString(text)
}

// Grounded reports whether one sentence may be rendered, given the records the
// assembler actually put in front of the model.
//
// A sentence is dropped WHOLE rather than trimmed. A sentence citing one real
// record and one invented one is a sentence whose claim may rest on the
// invented half, so keeping it with the good citation attached would present it
// as checked when it is not. An id in the prose is developer output whatever
// else the sentence says, and cutting the id out mid-clause leaves broken
// grammar the reader has to decode — every id the sentence needed is already in
// its evidence.
func Grounded(sentence Sentence, known map[Evidence]string) bool {
	return Ungrounded(sentence, known) == ""
}

// Ungrounded is Grounded with its reason, which is what a caller logging a drop
// needs and what a test asserting one reads. Empty means the sentence stands.
//
// The reason is a sentence a contact can act on — "names 14 August, which the
// records it cites do not" — because a silent drop is indistinguishable from a
// model that had nothing to say, and the two want opposite responses.
func Ungrounded(sentence Sentence, known map[Evidence]string) string {
	if strings.TrimSpace(sentence.Text) == "" || len(sentence.Evidence) == 0 {
		return "the sentence is empty or cites nothing"
	}
	if idInProse.MatchString(sentence.Text) {
		return "the prose spells a record id"
	}
	var cited strings.Builder
	for _, reference := range sentence.Evidence {
		// Keyed on the (kind, identity) PAIR, so a valid identity of the wrong
		// kind is still dropped. Stripped to identity() first: known is built
		// with no Name set, and comparing the full struct would fail a
		// name-bearing citation against a record it correctly cites.
		source, ok := known[identity(reference)]
		if !ok {
			return "it cites a record this answer was not written from"
		}
		cited.WriteString(source)
		cited.WriteString("\n")
	}
	// A citation is a POINTER: it proves the model named a real row, never that
	// the sentence says what the row says. So every date, amount and percentage
	// the sentence states has to be in the rows it points at — the connective
	// prose, which is the reading, stays free.
	if missing := specifics.Missing(sentence.Text, cited.String()); len(missing) > 0 {
		return "it names " + specifics.Texts(missing) + ", which the records it cites do not"
	}
	return ""
}

// Keep filters a batch to the sentences that survive Grounded, normalises any
// nature the vocabulary does not define down to fact, and collapses repeated
// citations.
//
// An unknown or absent nature reduces to FACT rather than being forwarded: a
// surface that passed the model's own string through would render a label the
// contract does not define, and the strictest reading is the safe one.
//
// knownNature is passed in rather than fixed here because each surface derives
// it from its own contract enum — deriving beats re-spelling, and a rename
// upstream should fail to compile rather than launder a hand-typed string.
func Keep(sentences []Sentence, known map[Evidence]string, knownNature map[string]bool, fact string) []Sentence {
	kept := make([]Sentence, 0, len(sentences))
	for _, sentence := range sentences {
		if !Grounded(sentence, known) {
			continue
		}
		if !knownNature[sentence.Nature] {
			sentence.Nature = fact
		}
		kept = append(kept, sentence)
	}
	return Dedupe(kept)
}

// TerminateSentence renders a stored statement as one sentence: a closing full
// stop when the value ends without one, and the author's own terminator kept
// when it has one. Rewriting a "?" into a "." would be an edit to approved
// text, which none of these surfaces promises, and appending a second "."
// to a value that already ends with one renders "..". Exported so every
// surface that turns a stored field into a sentence — the brief, the
// dossier — terminates it the same way rather than re-deriving the rule and
// one of them missing the already-terminated case.
func TerminateSentence(value string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(value), ";:, ")
	if trimmed == "" {
		return trimmed
	}
	// "…" included: a statement that trails off already ends, and appending a
	// full stop to it renders "….".
	for _, terminator := range []string{".", "!", "?", "…"} {
		if strings.HasSuffix(trimmed, terminator) {
			return trimmed
		}
	}
	return trimmed + "."
}

// Dedupe collapses repeated citations within each sentence, keeping first-seen
// order.
//
// The same record cited twice renders as two identical chips the reader cannot
// tell apart, and clicking either goes to the same place. Every writer exits
// through this, so the wire shape does not depend on which one wrote the answer.
func Dedupe(sentences []Sentence) []Sentence {
	out := make([]Sentence, 0, len(sentences))
	for _, sentence := range sentences {
		seen := make(map[Evidence]bool, len(sentence.Evidence))
		unique := make([]Evidence, 0, len(sentence.Evidence))
		for _, cited := range sentence.Evidence {
			// Keyed on identity(), not the citation itself: two citations of
			// the same record must collapse into one chip even if only one
			// of them happened to carry the record's Name.
			key := identity(cited)
			if seen[key] {
				continue
			}
			seen[key] = true
			unique = append(unique, cited)
		}
		sentence.Evidence = unique
		out = append(out, sentence)
	}
	return out
}

// Quoted is values.Quoted, the one grounding rule, under the name the
// extractors here call it by.
func Quoted(text, quote string) bool { return values.Quoted(text, quote) }

// CollapseSpace is values.CollapseSpace.
func CollapseSpace(s string) string { return values.CollapseSpace(s) }

// Source renders one record as the text a sentence citing it is held against.
//
// The assemblers hand their inputs to the model as JSON, so the JSON is what
// the model actually saw — including every field's rendering choice, which is
// the point: a deal's amount reaches the prompt as major units, and a check
// reading the minor-unit integer beside it would admit the figure a hundred
// times too large that the rendering exists to prevent.
//
//craft:ignore naked-any every assembler's record types are its own, and this renders whatever it is handed exactly as the prompt did
func Source(record any) string {
	rendered, err := json.Marshal(record)
	if err != nil {
		// A record that cannot be rendered is a record no sentence can be
		// checked against, so it contributes NOTHING rather than everything:
		// sentences citing it lose their source and are dropped, which is the
		// safe direction for a filter whose failure mode is admitting an
		// invention. Unreachable for the inputs here — they are plain structs
		// of strings, numbers and slices — and left as a value rather than an
		// error so one unrenderable record cannot cost a reader the whole brief.
		return ""
	}
	return string(rendered)
}

// BriefBounds is what one brief may hold once grounded: how many sentences,
// and how many of them may be advice. The natures are the caller's own
// spellings, derived from its contract enum.
type BriefBounds struct {
	Sentences       int
	Recommendations int
	Natures         map[string]bool
	Fact            string
	Recommendation  string
	// Refuse drops a grounded recommendation this caller will not show, on top
	// of the promise KeepBrief refuses for every caller. Nil adds none.
	Refuse func(Sentence) bool
}

// KeepBrief grounds a brief's sentences and then bounds it. Grounding and
// refusal go first, so a recommendation the reader never sees cannot spend the
// budget a valid one needed. An unknown nature reads as fact, the strictest
// reading.
func KeepBrief(sentences []Sentence, known map[Evidence]string, bounds BriefBounds) []Sentence {
	out := make([]Sentence, 0, min(len(sentences), bounds.Sentences))
	recommendations := 0
	for _, sentence := range sentences {
		if len(out) == bounds.Sentences {
			break
		}
		if !bounds.Natures[sentence.Nature] {
			sentence.Nature = bounds.Fact
		}
		if !Grounded(sentence, known) {
			continue
		}
		if sentence.Nature == bounds.Recommendation {
			if recommendations == bounds.Recommendations || OpensWithWriterSubject(sentence.Text) ||
				(bounds.Refuse != nil && bounds.Refuse(sentence)) {
				continue
			}
			recommendations++
		}
		out = append(out, sentence)
	}
	return Dedupe(out)
}

// writerSubjects is, per shipped language, every subject that makes a
// recommendation the writer's or the reader's own promise. Vietnamese anh, chị,
// em and mình are absent: each also addresses the reader as "you".
var writerSubjects = map[textlang.Lang][]string{
	textlang.English:    {"i", "i'll", "i'm", "i've", "i'd", "we", "we'll", "we're", "we've", "we'd", "let's"},
	textlang.German:     {"ich", "wir"},
	textlang.Vietnamese: {"tôi", "chúng tôi"},
}

// OpensWithWriterSubject reports whether text opens with an I or a we, in any
// shipped language. A recommendation that does is a promise, "I'll send the
// agreement", where the reader needed an instruction, "Send the agreement".
//
// Every language is checked whatever the brief's own: a model can answer in the
// wrong one, and the subjects do not collide across them.
func OpensWithWriterSubject(text string) bool {
	words := leadingWords(text, 2)
	for _, subjects := range writerSubjects {
		for _, subject := range subjects {
			if opensWith(words, strings.Fields(subject)) {
				return true
			}
		}
	}
	return false
}

// leadingWords is text's first n words, lowercased, with quote marks and
// punctuation around them stripped and ’ read as '.
func leadingWords(text string, n int) []string {
	normalised := strings.ToLower(strings.ReplaceAll(text, "’", "'"))
	words := strings.FieldsFunc(normalised, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' && r != '-'
	})
	out := make([]string, 0, n)
	for _, word := range words {
		if len(out) == n {
			break
		}
		// Trimmed so an opening ' quote or a dash bullet cannot hide the word after it.
		if word = strings.Trim(word, "'-"); word != "" {
			out = append(out, word)
		}
	}
	return out
}

func opensWith(words, subject []string) bool {
	if len(words) < len(subject) {
		return false
	}
	for i, word := range subject {
		if words[i] != word {
			return false
		}
	}
	return true
}

// Prose is a brief's sentences as one block of text, which is what a
// certification case reads a token against.
func Prose(sentences []Sentence) string {
	texts := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		texts = append(texts, sentence.Text)
	}
	return strings.Join(texts, " ")
}

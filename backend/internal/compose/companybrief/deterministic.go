// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// The deterministic brief: the floor every deployment gets, and the shape
// the model lane is asked to rewrite rather than exceed.
//
// It states facts already on the page, in the order a rep reads them:
// what this account is, what is open, what is stuck, what happened last.
// It never infers — no "they seem interested", no "worth a call" — because
// a sentence nobody can check is worth less than the number it paraphrases.

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Deterministic writes the brief without a model. Every sentence cites the
// record it came from, exactly as the model path's do, so the card renders
// and behaves identically whichever wrote it.
func Deterministic(companyID string, in Input, lang string) []Sentence {
	say := companyPhrasesFor(lang)
	account := accountEvidence(companyID)
	sentences := make([]Sentence, 0, 4)

	sentences = append(sentences, Sentence{Text: identityLine(in, say), Evidence: account})

	if len(in.OpenDeals) > 0 {
		sentences = append(sentences, Sentence{
			Text:     pipelineLine(in, say),
			Evidence: leadDealEvidence(in),
		})
	}
	// One sentence per stalled deal, so the reader opens the one they mean
	// instead of picking between chips hanging off a joined list.
	sentences = append(sentences,
		perRecordSentences(stalledDeals(in), citeDeal, dealID,
			func(deal DealIn) string { return stalledLine(deal, say) })...)
	if len(in.Recent) > 0 {
		last := in.Recent[0]
		sentences = append(sentences, Sentence{
			Text:     lastTouchLine(last, say),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: last.ID}},
		})
	}
	if len(in.OpenTasks) > 0 {
		sentences = append(sentences, Sentence{
			Text: fmt.Sprintf(say.say(floor.TasksStarting),
				countPhrase(len(in.OpenTasks), say.say(floor.OpenTaskOne), say.say(floor.OpenTaskMany)), in.OpenTasks[0].Name),
			// Cites the task itself, so the reader can open the one named.
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.OpenTasks[0].ID}},
		})
	}
	// Then what the company IS. Same two-part shape the model lane is asked
	// for, so the card reads the same whichever wrote it.
	sentences = append(sentences, profileLines(in, account, say)...)
	return claims.Dedupe(sentences)
}

// deterministicProfileLines bounds the company half of the floor. Two
// statements say what a company does; eight is the profile card, which the
// reader can open underneath.
const deterministicProfileLines = 2

func profileLines(in Input, account []Evidence, say spoken) []Sentence {
	out := make([]Sentence, 0, deterministicProfileLines)
	for _, entry := range in.Profile {
		if len(out) == deterministicProfileLines {
			break
		}
		label, ok := say.noun(floor.ProfileLabels, entry.Field)
		if !ok {
			continue
		}
		// A stored value of nothing but punctuation reduces to nothing, and a
		// line reading "What they sell: ." is worse than no line.
		statement := claims.TerminateSentence(entry.Value)
		if statement == "" {
			continue
		}
		out = append(out, Sentence{
			Text:     fmt.Sprintf("%s: %s", label, statement),
			Evidence: account,
		})
	}
	return out
}

func identityLine(in Input, say spoken) string {
	parts := []string{in.Name}
	if in.Industry != "" {
		parts = append(parts, in.Industry)
	}
	if in.SizeBand != "" {
		parts = append(parts, fmt.Sprintf(say.say(floor.ContactsSuffix), in.SizeBand))
	}
	line := strings.Join(parts, ", ") + "."
	if in.ContactCount > 0 {
		// The score is reported with the contact count it was taken over, so
		// a strong number from one contact never reads like a broad
		// relationship.
		if in.ContactCount == 1 {
			line += fmt.Sprintf(say.say(floor.StrengthOverOne), in.Strength)
		} else {
			line += fmt.Sprintf(say.say(floor.StrengthOverContacts), in.Strength, in.ContactCount)
		}
	}
	return line
}

func pipelineLine(in Input, say spoken) string {
	line := countPhrase(len(in.OpenDeals), say.say(floor.OpenDealOne), say.say(floor.OpenDealMany))
	total, currency, ok := oneCurrencyTotal(in.OpenDeals)
	if ok && total > 0 {
		// Minor units are rendered as a plain major-unit figure; the card
		// formats money properly, and this text is the fallback.
		line += fmt.Sprintf(say.say(floor.WorthAbout), values.MajorUnits(total, currency), currency)
	}
	// The won total carries its OWN currency: the 360 converts it to the
	// workspace base at each deal's frozen close-time rate, which has no
	// relation to whatever the open deals are priced in. Labelling it with
	// the open currency reported a real figure under the wrong unit.
	if in.WonLifetime > 0 && in.WonCurrency != "" {
		line += fmt.Sprintf(say.say(floor.WonToDate), values.MajorUnits(in.WonLifetime, in.WonCurrency), in.WonCurrency)
	}
	return line + "."
}

// oneCurrencyTotal sums the open deals only when they all agree on a
// currency.
//
// Adding minor units across currencies produces a number that is not money
// in any of them, and labelling the result with whichever deal happened to
// come first states it as a fact. A mixed-currency account gets the deal
// COUNT and no total: the card converts and totals properly, and this text
// is the floor, so under-reporting is the only honest option here.
func oneCurrencyTotal(deals []DealIn) (total int64, currency string, ok bool) {
	for _, deal := range deals {
		if deal.AmountMinor == 0 {
			continue // an amountless deal contributes nothing, and no currency
		}
		if deal.Currency == "" {
			// An amount whose currency nobody recorded cannot be added to
			// anything: folded into a later deal's total it would be reported
			// as that currency, which is a figure this account never had.
			// (The deal_amount_currency_pair CHECK makes this unreachable
			// from the database; Input is a plain struct, and the total is
			// money.)
			return 0, "", false
		}
		if currency == "" {
			currency = deal.Currency
		}
		if deal.Currency != currency {
			return 0, "", false
		}
		total += deal.AmountMinor
	}
	return total, currency, currency != ""
}

func stalledDeals(in Input) []DealIn {
	stalled := make([]DealIn, 0, len(in.OpenDeals))
	for _, deal := range in.OpenDeals {
		if deal.Stalled {
			stalled = append(stalled, deal)
		}
	}
	return stalled
}

func dealID(deal DealIn) string { return deal.ID }

// leadDealEvidence cites the one deal a pipeline COUNT is anchored on: the
// first the account listed. The sentence names no deal, so it needs somewhere
// for the reader to start, and citing all of them was what produced a row of
// chips nobody could tell apart.
func leadDealEvidence(in Input) []Evidence {
	return []Evidence{{EntityType: citeDeal, EntityID: in.OpenDeals[0].ID}}
}

func stalledLine(deal DealIn, say spoken) string {
	return fmt.Sprintf(say.say(floor.StalledDeal), deal.Name)
}

// kindNoun names an activity kind as a whole noun phrase in the reader's
// language — article and all where the language has one.
//
// English chose the article from the first letter, which is English grammar and
// nothing else: German picks by gender (eine E-Mail, ein Anruf) and Vietnamese
// uses no article at all. A kind the table does not name renders as its stored
// key, which says only that something happened — the honest reading of a row
// this build has no word for.
func kindNoun(kind string, say spoken) string {
	noun, _ := say.noun(floor.KindNouns, kind)
	return noun
}

func lastTouchLine(last ActIn, say spoken) string {
	noun := kindNoun(last.Kind, say)
	when := shortDate(last.At, say)
	switch {
	case when != "" && last.Subject != "":
		// The subject is quoted rather than woven into the sentence: it is text
		// from outside the workspace, and it must read as theirs, not ours.
		return fmt.Sprintf(say.say(floor.LastContactFull), noun, when, last.Subject)
	case when != "":
		return fmt.Sprintf(say.say(floor.LastContactDated), noun, when)
	case last.Subject != "":
		return fmt.Sprintf(say.say(floor.LastContactSubject), noun, last.Subject)
	default:
		return fmt.Sprintf(say.say(floor.LastContactPlain), noun)
	}
}

// countPhrase renders a count with the noun it counts, from the singular and
// plural its language supplies.
//
// English formed the plural by appending "s", which is wrong in the two other
// languages the product ships: German inflects irregularly (Aufgabe/Aufgaben)
// and Vietnamese does not inflect at all. So the sentence comes from the table
// rather than from a rule.
func countPhrase(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return fmt.Sprintf(many, count)
}

// shortDate renders an RFC3339 instant the way a reader writes a date, and
// returns empty for one it cannot read.
//
// Empty means "say nothing about when": the caller drops the clause instead of
// printing a machine timestamp at the reader. The instants are formatted by
// this package's own folds, so an unreadable one is a defect upstream of here
// rather than a fact about the account — and the sentence around it is still
// true without the date. The layout is the language's own.
func shortDate(at string, say spoken) string {
	if at == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(say.say(floor.DateLayout))
}

// DeterministicSections is the floor in the shape the card renders: the same
// sentences, sorted into the questions they answer.
//
// It writes no `fit` section, and that absence is the honest one. Fit is a
// judgment about what this account is worth to US, and a judgment is exactly
// what a floor with no model cannot make. A heading over a restated fact would
// claim an assessment nobody performed.
func DeterministicSections(companyID string, in Input, lang string) []Section {
	say := companyPhrasesFor(lang)
	account := accountEvidence(companyID)
	sections := make([]Section, 0, 4)

	// What the company IS: its identity, then the curated statements a human
	// already accepted, quoted rather than paraphrased.
	snapshot := append([]Sentence{{Text: identityLine(in, say), Evidence: account}},
		profileLines(in, account, say)...)
	sections = append(sections, Section{Kind: sectionSnapshot, Sentences: snapshot})

	var health []Sentence
	if len(in.OpenDeals) > 0 {
		health = append(health, Sentence{Text: pipelineLine(in, say), Evidence: leadDealEvidence(in)})
	}
	health = append(health, perRecordSentences(stalledDeals(in), citeDeal, dealID,
		func(deal DealIn) string { return stalledLine(deal, say) })...)
	if len(health) > 0 {
		sections = append(sections, Section{Kind: sectionHealth, Sentences: claims.Dedupe(health)})
	}

	var activity []Sentence
	if len(in.Recent) > 0 {
		last := in.Recent[0]
		activity = append(activity, Sentence{
			Text:     lastTouchLine(last, say),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: last.ID}},
		})
	}
	if len(activity) > 0 {
		sections = append(sections, Section{Kind: sectionActivity, Sentences: activity})
	}

	if len(in.OpenTasks) > 0 {
		sections = append(sections, Section{Kind: sectionNextStep, Sentences: []Sentence{{
			Text: fmt.Sprintf(say.say(floor.TasksStarting),
				countPhrase(len(in.OpenTasks), say.say(floor.OpenTaskOne), say.say(floor.OpenTaskMany)), in.OpenTasks[0].Name),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.OpenTasks[0].ID}},
		}}})
	}
	return sections
}

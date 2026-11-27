// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"slices"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/namematch"
)

// legalSuffixes is PO-PARAM-1: the trailing tokens company-name
// normalization strips so "Acme Inc" and "Acme GmbH" both reduce to
// "acme" and meet at the fuzzy tier for a human to judge.
//
//nolint:goconst // legal-form words; "company" here is not the record kind.
var legalSuffixes = map[string]bool{
	"inc": true, "llc": true, "ltd": true, "gmbh": true, "ag": true,
	"sa": true, "sas": true, "bv": true, "oy": true, "plc": true,
	"co": true, "corp": true, "kg": true, "ug": true,
	// The same forms written in full, as reps type them as often as the short ones.
	"incorporated": true, "corporation": true, "company": true, "limited": true,
	"pty": true, "llp": true, "jsc": true, "nv": true,
	// The markets that write the form in front ALSO write one behind, and a name
	// carries whichever the writer used. Indonesia's listed companies end in
	// "Tbk" and bracket the brand between it and a leading "PT"; Romania and
	// Turkey put theirs at the end alone.
	//
	// EVERY ENTRY HERE MUST BE A WORD NO COMPANY IS CALLED, because this map
	// feeds NormalizeCompanyName, which is a stored grouping key: a word wrongly
	// listed does not merely inflate a score, it files two unrelated companies
	// under one key. Measured, "zoo" (the Polish "z o.o.") made "San Diego Zoo"
	// and "San Diego" the same key, and "as" took the last word off "Trading
	// As". Both are out; the Polish and Nordic forms are not worth that.
	"srl": true, "sro": true, "tbk": true, "oyj": true, "sti": true,
}

// legalConnectives join the halves of a COMPOUND legal form: "GmbH & Co. KG"
// is one form, and a strip that halted on the ampersand left "basecom gmbh &"
// as the key — a name no account is stored under.
//
// They are NOT suffixes in their own right, and treating them as such was
// wrong: it collapsed "Research and" to "research" and "Miller und" to
// "miller", so two unrelated accounts could meet at the same key. A connective
// is consumed only when a real suffix has already been stripped and another
// one follows it — which is exactly the compound case and nothing else.
var legalConnectives = map[string]bool{"&": true, "und": true, "and": true}

// NormalizeContactName is the contact-name key: namematch.Fold plus a
// collapse of internal whitespace. Both halves are
// load-bearing and neither implies the other.
//
// The fold is what makes Straße and STRASSE one contact — strings.ToLower
// leaves ß alone, and the DACH market meets that pair daily. The collapse is
// what makes a page that reflows "Anna  Muster" across a line break the same
// contact as "Anna Muster" — a key that kept the second space mints a second
// record on the next crawl of an unchanged page.
//
// Not namematch.Fold itself, which also feeds nameSimilarity. That input is
// pinned by PO-PARAM-JW-2 so the spec's worked examples stay reproducible.
func NormalizeContactName(s string) string { return foldedNameKey(s) }

// foldedNameKey is the key both dedupe lanes compare on: namematch.Fold, no
// ASCII apostrophe, and a collapse of internal whitespace.
//
// EXACTLY the ASCII apostrophe, because that is what the SQL arm in front of
// this removes. Folding the typographic one as well would make the key call
// equal a pair that arm does not admit, and the arm owes reachability for every
// Go-equal pair — TestTheNameKeyArmAdmitsEveryGoEqualPair fails in that
// direction, which is the one that loses a duplicate silently.
//
// One helper because the contact lane and the company lane ask one question.
func foldedNameKey(s string) string {
	return collapseSpaces(strings.ReplaceAll(namematch.Fold(s), "'", ""))
}

// companyNamesAreTheSame answers whether two names are one name. Only case,
// accents and spacing are folded. A legal suffix stays, for the reason
// CompanyCandidateScore.ExactName gives.
func companyNamesAreTheSame(left, right string) bool {
	return sameNameKey(foldedNameKey(left), foldedNameKey(right))
}

// sameNameKey says two folded name keys name one name. An empty key names none.
func sameNameKey(left, right string) bool { return left != "" && left == right }

// collapseSpaces reduces every run of whitespace to one space. A name read off
// a web page carries the spacing of its markup, and "Baqend  GmbH" is not a
// different company from "Baqend GmbH".
func collapseSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }

// digraphNameKey is the contact-name key with umlauts written as digraphs, so
// "Björn" and "Bjoern" share a key while "Bjoern" and "Bjorn" do not. It sits
// beside NormalizeContactName and never replaces it. That key also feeds
// locks, stored review hashes and other lanes, none of which may move.
func digraphNameKey(s string) string {
	return foldedNameKey(namematch.Digraphs(s))
}

// contactNameSimilarity is nameSimilarity for the contact lane, taking the
// better of the plain and the digraph spelling. Only the contact score asks
// it: lead dedupe and site-contact routing keep nameSimilarity as it is.
func contactNameSimilarity(a, b string) float64 {
	return contactFormsSimilarity(contactNameFormsOf(a), contactNameFormsOf(b))
}

// contactNameForms is a contact name in every form the contact score compares.
type contactNameForms struct {
	folded, digraphs []rune
	given, family    string
	// numbers are the name's runs of digits, sorted, without leading zeros.
	numbers []string
	// noDigraph says the digraph form is the folded one.
	noDigraph bool
}

func contactNameFormsOf(name string) contactNameForms {
	given, family := namematch.SplitName(name)
	forms := contactNameForms{
		folded: namematch.ScoringRunes(namematch.Fold(name)), digraphs: namematch.ScoringRunes(namematch.Fold(namematch.Digraphs(name))),
		given: given, family: family, numbers: nameNumbers(name),
	}
	forms.noDigraph = slices.Equal(forms.folded, forms.digraphs)
	return forms
}

// nameNumbers are the digit runs of a name, "01" and "1" one number.
func nameNumbers(name string) []string {
	runs := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsDigit(r) })
	for i, run := range runs {
		if trimmed := strings.TrimLeft(run, "0"); trimmed != "" {
			runs[i] = trimmed
		} else {
			runs[i] = "0"
		}
	}
	slices.Sort(runs)
	return runs
}

// contactFormsSimilarity is contactNameSimilarity over forms. Two names with
// no umlaut have digraph forms equal to their folded ones, scored once.
//
// Names that both carry numbers and disagree on them ("Bulk 01", "Bulk 02") are
// two numbered records. One digit barely moves Jaro-Winkler, so they score 0.
func contactFormsSimilarity(a, b contactNameForms) float64 {
	if len(a.numbers) > 0 && len(b.numbers) > 0 && !slices.Equal(a.numbers, b.numbers) {
		return 0
	}
	plain := namematch.JaroWinklerRunes(a.folded, b.folded)
	if a.noDigraph && b.noDigraph {
		return plain
	}
	return max(plain, namematch.JaroWinklerRunes(a.digraphs, b.digraphs))
}

// NormalizeCompanyName is namematch.Fold plus the PO-PARAM-1 legal-suffix
// strip, applied only to the trailing token. "Co" inside "Coca Co" is a name,
// and "Co" at the end is a suffix.
//
// The strip never consumes the whole name: "Co" alone stays "co", because a
// company may BE its suffix and an empty key would collide with every other
// empty key.
func NormalizeCompanyName(s string) string {
	fields := strings.Fields(namematch.Fold(strings.ReplaceAll(s, ",", " ")))
	strippedOne := false
	for len(fields) > 1 {
		last := undottedForm(fields[len(fields)-1])
		switch {
		case legalSuffixes[last]:
			strippedOne = true
		case legalConnectives[last] && strippedOne && len(fields) > 2 &&
			legalSuffixes[undottedForm(fields[len(fields)-2])]:
			// A connective BETWEEN two legal forms — the "GmbH & Co. KG"
			// case. Consumed only here, so a company whose name simply ends
			// in "and" keeps it.
		default:
			return strings.Join(fields, " ")
		}
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}

// companyLegalForm is the legal form NormalizeCompanyName strips from a name,
// undotted and space-joined, or "" when it strips none: "Acme GmbH & Co. KG"
// reads "gmbh & co kg", and "Acme" reads "".
func companyLegalForm(s string) string {
	fields := strings.Fields(namematch.Fold(strings.ReplaceAll(s, ",", " ")))
	core := strings.Fields(NormalizeCompanyName(s))
	if len(core) >= len(fields) {
		return ""
	}
	tail := fields[len(core):]
	for i, word := range tail {
		tail[i] = undottedForm(word)
	}
	return strings.Join(tail, " ")
}

// undottedForm is a trailing word with its internal dots removed, so "S.R.L."
// and "SRL" are one Romanian form. Trimming only the final dot left "s.r.l." as
// one token matching no entry.
//
// Only for the lookup: a word that turns out not to be a legal form keeps its
// dots.
func undottedForm(word string) string {
	if !strings.Contains(word, ".") {
		return word
	}
	return strings.ReplaceAll(word, ".", "")
}

// nameSimilarity is `name_sim`: Jaro-Winkler over normalized input,
// in [0,1].
func nameSimilarity(a, b string) float64 {
	return namematch.JaroWinkler(namematch.Fold(a), namematch.Fold(b))
}

// exactNameKeySQL is the SQL side of the Go exact-name comparison, and BOTH
// dedupe lanes ask it of both halves of their equality arm, so a comparison
// cannot fold its two sides with different normalizations of one name.
//
// One function because there is one question. NormalizeContactName is
// namematch.Fold plus a whitespace collapse. companyNamesAreTheSame folds the
// same three things: case, accents and spacing. The company lane's
// legal-suffix key is a DIFFERENT question and is not this one. See
// CompanyCandidateScore.ExactName for why "Baqend GmbH" and "Baqend Inc" must
// not fold together.
//
// Not a mirror of the Go key — it deliberately is not one. SQL's lower and Go's
// Unicode full fold are different normalizations, and the arm exists so the lane
// does not rely on either being a superset of the other. What it DOES owe is
// every pair the Go key calls equal, since that is the equality the lane
// decides on, and three properties of the Go key are not free in SQL:
//
//   - the trim, and the collapse of internal whitespace, because a name a
//     crawled page reflowed across a line break is Go-equal to the same name
//     typed by hand;
//   - Greek final sigma, which full folding maps onto σ and lower() leaves
//     alone, so "Οδυσσεύς" and "ΟΔΥΣΣΕΥΣ" are one contact in Go and two here.
//
// Held by TestTheNameKeyArmAdmitsEveryGoEqualPair (creatededupe_integration_test.go),
// which runs this expression against the database over the same pairs the
// reachability test carries — the arm's own claim, asked of the arm rather than
// of the query it sits in, where the trigram arm answers first and hides it.
func exactNameKeySQL(expr string) string {
	return storekit.SQLf(
		`btrim(regexp_replace(replace(f_fold_apostrophes(lower(%s)), 'ς', 'σ'), '\s+', ' ', 'g'))`, expr)
}

// familyNameKeySQL is the last token of the folded name, or of the part before a
// comma ("Doe, Jon" is Jon Doe). idx_contact_family_name_key holds the same
// expression. The employer arm of the candidate query reads colleagues by that
// index and never scans all of an employer's contacts.
func familyNameKeySQL(expr string) string {
	return storekit.SQLf(`substring(%s from '[^ ]+$')`,
		exactNameKeySQL(storekit.SQLf(`CASE WHEN %[1]s LIKE '%%,%%' THEN split_part(%[1]s, ',', 1) ELSE %[1]s END`, expr)))
}

// exactDigraphNameKeySQL is exactNameKeySQL over the name with its umlauts
// written as digraphs: the SQL side of digraphNameKey. It is its own arm of the
// contact candidate query, so a "Björn"/"Bjoern" pair reaches the name lane
// whatever the trigram arm makes of it. It composes to NFC first, as the Go key
// does, so an "o" followed by a combining diaeresis is an "ö" here too.
func exactDigraphNameKeySQL(expr string) string {
	return exactNameKeySQL(storekit.SQLf(
		`replace(replace(replace(normalize(lower(%s), NFC), 'ä', 'ae'), 'ö', 'oe'), 'ü', 'ue')`, expr))
}

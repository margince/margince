// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser spells the company-profile vocabulary five more times, and every
// one of them fails SILENTLY when it falls short.
//
// companyprofilevocabulary_test.go holds the Go mirrors against the
// organization_profile_field CHECK. It cannot read TypeScript, so the five
// below were unheld — and all five were genuinely missing three fields while
// that gate stayed green. What each miss does to a person:
//
//   - COLD_FIELD_LABELS and PROFILE_FIELD_LABELS both fall back to the raw
//     column with its underscores spaced out, so a missing entry renders
//     "legal form" in lower case beside properly named fields.
//   - LEGAL_IDENTITY_FIELDS groups the fields a section shows; a missing entry
//     is a field no screen ever offers.
//   - MANUAL_QUESTIONS is the interview's own list. Adding an i18n key does
//     NOT add the question, so a missing entry leaves the copy translated in
//     three locales and rendered nowhere.
//   - onboardingDraftPayload serializes the answers for the resume checkpoint
//     and the conversational draft; a missing entry is an answer the reader
//     gives and is then asked for again with the field blank.
//
// Two of those shipped exactly that way. The frontend suite caught the label
// maps only because a test happened to assert rendered label text, and the
// interview only because the i18n orphan check noticed keys nothing rendered.
//
// It reads the sources with a regexp rather than a TypeScript parser, the same
// trade frontendminorunits_test.go makes for the same reason: the alternative
// is a second copy of the vocabulary maintained here, which is the defect this
// gate exists to catch.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// profileVocabularyScreens are the browser's mirrors of the vocabulary: the
// file, the declaration the fields are listed inside, and what a reader loses
// when one falls short.
var profileVocabularyScreens = []struct {
	// file is the source, relative to backend/gates.
	file string
	// opens is the text the declaration starts with. The literal that follows
	// it is what this gate reads.
	opens string
	// closes ends that literal.
	closes string
	// why states the consequence, so a failure explains itself.
	why string
	// omits are fields this mirror may legitimately lack, each with its reason.
	omits map[string]string
}{
	{
		file:   "../frontend/src/screens/common.tsx",
		opens:  "const COLD_FIELD_LABELS: Record<string, MessageKey> = {",
		closes: "};",
		why:    "the field renders as its own column name with the underscores spaced out",
	},
	{
		file:   "../frontend/src/screens/organizations.tsx",
		opens:  "const PROFILE_FIELD_LABELS: Record<string, MessageKey> = {",
		closes: "};",
		why:    "the company record draws the field as its own column name",
	},
	{
		file:   "../frontend/src/screens/onboarding.tsx",
		opens:  "export const LEGAL_IDENTITY_FIELDS = [",
		closes: "] as const;",
		why:    "no section carries the field, so no screen ever offers it",
		// This group is the legal block alone; the rest of the vocabulary is
		// grouped beside it under OFFER_FIELDS, CUSTOMER_FIELDS and
		// SALES_FIELDS. The union is what must be whole, which the payload
		// mirror below reads.
		omits: onlyTheLegalBlockOmits(),
	},
	{
		file:   "../frontend/src/screens/onboarding.tsx",
		opens:  "export function onboardingDraftPayload(values: CompanyForm) {",
		closes: "\n}",
		why:    "the answer is dropped from the saved draft, so a resumed interview asks for it again with the field blank",
	},
	{
		file:   "../frontend/src/screens/onboarding-manual-interview.tsx",
		opens:  "const MANUAL_QUESTIONS: readonly ManualQuestion[] = [",
		closes: "\n];",
		why:    "the interview never asks for the field, however many locales its question is translated into",
	},
}

// onlyTheLegalBlockOmits names the fields LEGAL_IDENTITY_FIELDS does not
// carry, because a sibling group does.
//
// Listed rather than derived, and that is a compromise worth stating: the
// three sibling groups sit in the same file and could be read the same way.
// They are not, because this gate would then assert only that four lists
// partition one vocabulary — true of any partition, including one that put
// the register court under SALES_FIELDS. The union is held by the payload
// mirror above, which reads every field the interview collects.
//
// The one thing this list cannot catch is its own staleness: a field dropped
// from a sibling group stays exempt here. `frontend/src/screens/onboarding.test.tsx`
// closes that, comparing the four groups' union against onboardingDraftPayload
// — so the two together hold what neither holds alone, and the frontend half
// is the one that fails when a group loses a field.
func onlyTheLegalBlockOmits() map[string]string {
	const elsewhere = "grouped under OFFER_FIELDS, CUSTOMER_FIELDS or SALES_FIELDS instead"
	omits := map[string]string{}
	for _, field := range []string{
		"offer_summary", "icp", "value_proposition", "usp",
		"customer_pains", "desired_outcomes", "buying_center",
		"buying_intents", "common_objections", "sales_motion",
	} {
		omits[field] = elsewhere
	}
	return omits
}

// tsVocabularyField reads a vocabulary member where a mirror can NAME one and
// mean it: an object key (`register_court:` or `"register_court":`), or a
// standalone string in an array or property (`"register_court"`).
//
// It deliberately does not match a bare identifier anywhere in the text. The
// first version did, and it read the field name out of the i18n key BESIDE
// each entry — `register_court: "ob.field.register_court"` — so deleting the
// whole line left the gate green, which is precisely the failure this gate
// was written to end. Mutation-check any change here against all five
// mirrors; a looser pattern passes for the wrong reason.
//
// Mutation-check with `go test -count=1`. The sources this reads are not Go
// files, so the test cache does not notice when one changes and a stale PASS
// looks exactly like a real one.
//
// Two shapes it would read wrongly, neither present in the five mirrors and
// both worth knowing before adding a sixth:
//
//   - A backtick string (“ `register_court` “) is not matched, so an entry
//     spelled that way reads as absent — a false failure, which is the safe
//     direction and says so loudly.
//   - A field name inside a longer string ("Ask about register_court") can
//     match, so a mirror whose prose quotes a field could keep this gate green
//     after the real entry is deleted. That is the unsafe direction, and it is
//     why literalAfter strips comments and why every mirror added here is
//     mutation-checked entry by entry rather than trusted to the pattern.
var tsVocabularyField = regexp.MustCompile(`(?m)(?:^|[\s,{[])["']?([a-z][a-z_]*[a-z])["']?\s*(?::|,|$|])`)

// tsVocabularyComment strips comments before the fields are read, so a field
// NAMED in prose beside the literal cannot stand in for a real entry.
var tsVocabularyComment = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

func TestTheBrowserSpellsTheWholeProfileVocabulary(t *testing.T) {
	t.Parallel()
	admitted := tableCheckSets(t)[theProfileVocabulary]
	if len(admitted) < 10 {
		t.Fatalf("only %d value(s) derived for %s — the migration scan has stopped reading it",
			len(admitted), theProfileVocabulary)
	}

	for _, mirror := range profileVocabularyScreens {
		source, err := os.ReadFile(mirror.file)
		if err != nil {
			t.Fatalf("reading %s: %v", mirror.file, err)
		}
		literal, err := literalAfter(string(source), mirror.opens, mirror.closes)
		if err != nil {
			t.Fatalf("%s: %v — this gate is reading a shape that is gone", mirror.file, err)
		}
		named := map[string]bool{}
		for _, match := range tsVocabularyField.FindAllStringSubmatch(literal, -1) {
			named[match[1]] = true
		}
		if len(named) == 0 {
			t.Errorf("%s: no fields parsed out of %s — a mirror that reads as empty agrees with everything",
				mirror.file, strings.TrimSuffix(mirror.opens, " = {"))
			continue
		}

		var missing []string
		for _, field := range admitted {
			if named[field] || mirror.omits[field] != "" {
				continue
			}
			missing = append(missing, field)
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("%s omits %s, which %s admits.\n"+
				"  Consequence: %s.\n"+
				"  Add it there, or take it out of the CHECK if the product does not have it.",
				mirror.file, strings.Join(missing, ", "), theProfileVocabulary, mirror.why)
		}
	}
}

// literalAfter returns the text between a declaration's opening and the token
// that closes it.
func literalAfter(source, opens, closes string) (string, error) {
	start := indexAfter(source, opens)
	if start < 0 {
		return "", errNoSuchDeclaration(opens)
	}
	end := indexAfter(source[start:], closes)
	if end < 0 {
		return "", errUnterminated(opens)
	}
	return tsVocabularyComment.ReplaceAllString(source[start:start+end], " "), nil
}

type errNoSuchDeclaration string

func (e errNoSuchDeclaration) Error() string {
	return "no declaration opening with " + string(e)
}

type errUnterminated string

func (e errUnterminated) Error() string {
	return "the literal opening with " + string(e) + " is unterminated"
}

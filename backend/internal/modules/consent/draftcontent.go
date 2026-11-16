// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What an advertising draft does not meet among the content rules of its
// recipients' packs, asked while it is written. A finding is advice to the
// writer and never refuses the send.

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The draft findings, in the vocabulary of the preview contract.
const (
	// DraftClaimWithoutProof is a claim a pack allows only beside a stated
	// proof, made in a draft that states none.
	DraftClaimWithoutProof = "claim_without_proof"
	// DraftLanguageFirst is a draft that does not open in the language a pack
	// asks advertising to lead with.
	DraftLanguageFirst = "advertising_language_first"
)

// DraftFinding is one advertising content rule a draft does not meet.
type DraftFinding struct {
	Requirement string
	Detail      string
	// Words are the claims found, for DraftClaimWithoutProof.
	Words []string
	// Language is the language asked for, for DraftLanguageFirst.
	Language string
}

// CheckAdvertisingDraft answers what an advertising draft to recipients does
// not meet. Recipients whose country no pack covers add nothing.
func (s *Store) CheckAdvertisingDraft(ctx context.Context, recipients []string, subject, body string) ([]DraftFinding, error) {
	// The grant the send needs: the answer is about a message this caller is
	// writing, and it reads each recipient's country to give it.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return nil, err
	}
	var findings []DraftFinding
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, address := range recipients {
			country, err := recipientCountryOf(ctx, tx, connector.Recipient{Email: address})
			if err != nil {
				return err
			}
			rules, _, found := applicableRules(country.Code)
			if !found {
				continue
			}
			for _, f := range draftFindings(rules.DisclosureLanguage, rules.ClaimsNeedProof, subject, body) {
				if !slices.ContainsFunc(findings, func(have DraftFinding) bool { return have.Detail == f.Detail }) {
					findings = append(findings, f)
				}
			}
		}
		return nil
	})
	return findings, err
}

// draftFindings checks one draft against one pack's content rules.
func draftFindings(language string, claims []string, subject, body string) []DraftFinding {
	var out []DraftFinding
	text := norm.NFC.String(subject + "\n" + body)
	if found := claimsIn(text, claims); len(found) > 0 && !statesProof(text) {
		out = append(out, DraftFinding{
			Requirement: DraftClaimWithoutProof,
			Detail: fmt.Sprintf("the draft claims %q; state the proving document's name, number and date beside it",
				strings.Join(found, `", "`)),
			Words: found,
		})
	}
	if textlang.Known(language) && !leadsWith(norm.NFC.String(body), textlang.Lang(language)) {
		out = append(out, DraftFinding{
			Requirement: DraftLanguageFirst,
			Detail: fmt.Sprintf("the draft does not open in %q; write it in that language first, "+
				"with any other language below it", language),
			Language: language,
		})
	}
	return out
}

// claimsIn answers each claim word text uses as a whole word, ignoring case.
// A longer claim takes its words with it, so "tốt nhất" is not also reported
// as "nhất".
func claimsIn(text string, claims []string) []string {
	lowered := []rune(strings.ToLower(text))
	ordered := slices.Clone(claims)
	slices.SortStableFunc(ordered, func(a, b string) int { return len([]rune(b)) - len([]rune(a)) })
	var found []string
	for _, claim := range ordered {
		word := []rune(strings.ToLower(norm.NFC.String(claim)))
		if at := wholeWordAt(lowered, word); at >= 0 {
			found = append(found, claim)
			for i := range word {
				lowered[at+i] = ' '
			}
		}
	}
	return found
}

func wholeWordAt(text, word []rune) int {
	for at := 0; at+len(word) <= len(text); at++ {
		if !slices.Equal(text[at:at+len(word)], word) {
			continue
		}
		before := at == 0 || !unicode.IsLetter(text[at-1])
		after := at+len(word) == len(text) || !unicode.IsLetter(text[at+len(word)])
		if before && after {
			return at
		}
	}
	return -1
}

var (
	dateInText  = regexp.MustCompile(`\b(\d{1,2}[./-]\d{1,2}[./-]\d{4}|\d{4}-\d{2}-\d{2})\b`)
	digitInText = regexp.MustCompile(`\d`)
)

// statesProof reports whether a line of text holds a date and a document
// number, as in "Giấy chứng nhận số 123/2026/GCN ngày 01/02/2026".
func statesProof(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		date := dateInText.FindStringIndex(line)
		if date == nil {
			continue
		}
		rest := line[:date[0]] + line[date[1]:]
		if digitInText.MatchString(rest) && strings.IndexFunc(rest, unicode.IsLetter) >= 0 {
			return true
		}
	}
	return false
}

// leadsWith reports whether the first paragraph whose language can be told is
// in language. A draft with no such paragraph is not judged.
func leadsWith(body string, language textlang.Lang) bool {
	for _, paragraph := range strings.Split(body, "\n\n") {
		if found := textlang.Detect(paragraph); found != textlang.Unknown {
			return found == language
		}
	}
	return true
}

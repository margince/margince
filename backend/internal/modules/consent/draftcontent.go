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
	"github.com/margince/margince/backend/internal/platform/salutation"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// The draft findings, in the vocabulary of the preview contract.
const (
	// DraftClaimWithoutProof is a claim a pack allows only beside a stated
	// proof, made in a draft that states none.
	DraftClaimWithoutProof = "claim_without_proof"
	// DraftLanguageFirst is a draft that does not open in the language a pack
	// asks advertising to lead with.
	DraftLanguageFirst = "advertising_language_first"
	// DraftForeignAdvertiser is advertising sent for an entity established
	// outside the recipient's country. That country's pack asks such an
	// advertiser to go through a local advertising business.
	DraftForeignAdvertiser = "foreign_advertiser"
)

// DraftFinding is one advertising content rule a draft does not meet.
type DraftFinding struct {
	Requirement string
	Detail      string
	// Words are the claims found, for DraftClaimWithoutProof.
	Words []string
	// Language is the language asked for, for DraftLanguageFirst.
	Language string
	// Country is the recipient's country, for DraftForeignAdvertiser.
	Country string
}

// CheckAdvertisingDraft answers what an advertising draft to recipients does
// not meet. Recipients whose country no pack covers add nothing.
func (s *Store) CheckAdvertisingDraft(ctx context.Context, recipients []string, countries []commsauthz.CountryOverride, subject, body string) ([]DraftFinding, error) {
	// The grant the send needs: the answer is about a message this caller is
	// writing, and it reads each recipient's country to give it.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return nil, err
	}
	var findings []DraftFinding
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// One check per pack, however many recipients share it.
		var codes []jurisdiction.Code
		for _, address := range recipients {
			country, err := recipientCountryStated(ctx, tx, connector.Recipient{Email: address}, countries)
			if err != nil {
				return err
			}
			if !slices.Contains(codes, country.Code) {
				codes = append(codes, country.Code)
			}
		}
		sender, err := s.senderCountryTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, code := range codes {
			for _, f := range packDraftFindings(code, sender, subject, body) {
				if !slices.ContainsFunc(findings, func(have DraftFinding) bool { return have.Detail == f.Detail }) {
					findings = append(findings, f)
				}
			}
		}
		return nil
	})
	return findings, err
}

// packDraftFindings checks one draft against the content rules of the pack of
// country code, for a sending entity established in sender. No pack, no finding.
func packDraftFindings(code, sender jurisdiction.Code, subject, body string) []DraftFinding {
	rules, _, found := applicableRules(code)
	if !found {
		return nil
	}
	findings := draftFindings(contentRules{
		language: rules.DisclosureLanguage, claims: rules.ClaimsNeedProof,
	}, subject, body)
	if rules.ForeignAdvertiserNeedsLocalAgency && sender != code {
		findings = append(findings, foreignAdvertiser(code))
	}
	return findings
}

// senderCountryTx is the country of the legal entity the caller sends for,
// the one the data-protection selector reads.
func (s *Store) senderCountryTx(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error) {
	entity, err := sendingEntityOf(ctx, tx)
	if err != nil {
		return "", err
	}
	code, err := s.dataProtectionCountryTx(ctx, tx, entity)
	return jurisdiction.Code(strings.ToLower(string(code))), err
}

// foreignAdvertiser is the notice for advertising into country from a sending
// entity established elsewhere. The customer answers for it.
func foreignAdvertiser(country jurisdiction.Code) DraftFinding {
	return DraftFinding{
		Requirement: DraftForeignAdvertiser,
		Detail: fmt.Sprintf("the sending entity is not established in %q; an advertiser from elsewhere that does "+
			"not operate there must advertise through a local advertising service business, and you answer for it",
			string(country)),
		Country: string(country),
	}
}

// contentRules are the fields of one pack's rules a draft is checked against.
type contentRules struct {
	language string
	claims   []string
}

// draftFindings checks one draft against one pack's content rules.
func draftFindings(rules contentRules, subject, body string) []DraftFinding {
	var out []DraftFinding
	language := rules.language
	text := norm.NFC.String(subject + "\n" + body)
	if found := claimsIn(text, rules.claims); len(found) > 0 && !statesProof(text) {
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
// A sign-off line ("Best,\nAn") ranks nothing and is skipped. Longer claims
// are matched first and blanked out wherever they stand, so "tốt nhất" is not
// also reported as "nhất".
func claimsIn(text string, claims []string) []string {
	lines := slices.DeleteFunc(strings.Split(text, "\n"), func(line string) bool {
		return salutation.IsClosing(strings.TrimSpace(line))
	})
	lowered := strings.ToLower(strings.Join(lines, "\n"))
	ordered := slices.Clone(claims)
	slices.SortStableFunc(ordered, func(a, b string) int { return len(b) - len(a) })
	var found []string
	for _, claim := range ordered {
		var hit bool
		if lowered, hit = blankWholeWords(lowered, strings.ToLower(norm.NFC.String(claim))); hit {
			found = append(found, claim)
		}
	}
	return found
}

// blankWholeWords replaces every whole-word occurrence of word in text with
// spaces of the same byte length, and reports whether there was one. Word
// edges are the ones the draft checks use, so a digit continues a word.
func blankWholeWords(text, word string) (string, bool) {
	if word == "" {
		return text, false
	}
	hit := false
	for offset := 0; offset < len(text); {
		i := strings.Index(text[offset:], word)
		if i < 0 {
			break
		}
		start, end := offset+i, offset+i+len(word)
		if salutation.WordBoundary(text, start-1) && salutation.WordBoundary(text, end) {
			text = text[:start] + strings.Repeat(" ", len(word)) + text[end:]
			hit = true
		}
		offset = start + 1
	}
	return text, hit
}

var (
	dateInText  = regexp.MustCompile(`\b(\d{1,2}[./-]\d{1,2}[./-]\d{4}|\d{4}-\d{2}-\d{2})\b`)
	digitInText = regexp.MustCompile(`\d`)
)

// statesProof reports whether a line of text holds a date and a document
// number, as in "Giấy chứng nhận số 123/2026/GCN ngày 01/02/2026".
func statesProof(text string) bool {
	for line := range strings.SplitSeq(text, "\n") {
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
	found := textlang.DetectFirst(strings.Split(body, "\n\n")...)
	return found == textlang.Unknown || found == language
}

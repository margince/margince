// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Whether a website speaks for the company it was read for. A domain can change
// hands, and a lapsed one may now serve a casino or a spam shop. That site's
// name, text, logo and staff describe another business. Domain triage and the
// deep read's own apply ask SiteSpeaksFor, because no human checks their values.
// A human who confirms exact values (onboarding, cold start, an approval) is
// the check on those paths.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/namematch"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanyIdentity is what the workspace already calls a company: its names and
// its domains. A triage read knows only the domain it asks about.
type CompanyIdentity struct {
	Names   []string
	Domains []string
}

// SiteSpeaksFor reports whether a site may name and describe a company. One of
// the names the site gives itself must account for a name or domain label of
// the company. A site that states no name claims no identity, so it passes.
func SiteSpeaksFor(stated []string, known CompanyIdentity) bool {
	claimed := false
	for _, name := range stated {
		if strings.TrimSpace(name) == "" {
			continue
		}
		claimed = true
		if nameAccountsFor(name, known) {
			return true
		}
	}
	return !claimed
}

func nameAccountsFor(stated string, known CompanyIdentity) bool {
	for _, domain := range known.Domains {
		if label := freemail.RegistrableLabel(domain); label != "" && statedNameSpellsLabel(stated, label) {
			return true
		}
	}
	for _, name := range known.Names {
		// Both ways round: "Acme" agrees with "Acme Robotics GmbH", and so does
		// "IBM" with "International Business Machines".
		for _, storedKey := range compactNames(name) {
			for _, statedKey := range compactNames(stated) {
				if statedNameSpellsLabel(stated, storedKey) || statedNameSpellsLabel(name, statedKey) {
					return true
				}
			}
		}
	}
	return false
}

// genericBrandWords are words that many unrelated companies share, in English,
// German and Vietnamese, folded. Sharing one says nothing about being the same
// business, so they never match alone.
var genericBrandWords = wordSet(`group solutions services service consulting tech digital global holding
	holdings international systems partners partner gruppe losungen loesungen dienstleistungen beratung
	technik technologie technologien systeme gmbh cong tnhh phan doan dich giai phap nghe quoc toan thong`)

// initialConnectives are the small words a name's initials may leave out, keep
// as a letter, or keep whole. "The Boston Consulting Group" is bcg, and "Bank of
// America" is bofa.
var initialConnectives = wordSet("the of and und der die das va cua")

// wordSet reads a list of words separated by spaces.
func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for w := range strings.FieldsSeq(words) {
		set[w] = true
	}
	return set
}

// statedNameSpellsLabel reports whether a company name accounts for a domain
// label. "Kestner Consulting" spells kestnerconsulting, and "Marsh Advisory"
// spells rowanmarsh through its word "marsh". "Bayerische Motoren Werke AG"
// spells bmw through its initials, and "Müller GmbH" spells mueller.
func statedNameSpellsLabel(stated, label string) bool {
	for _, foldedLabel := range brandFolds(label) {
		key := strings.Join(brandWords(foldedLabel), "")
		if key == "" {
			return true
		}
		for _, folded := range brandFolds(stated) {
			if wordsSpellLabel(folded, key) {
				return true
			}
		}
	}
	return false
}

// wordsSpellLabel is statedNameSpellsLabel for one folding of each side. A
// label under four runes counts only as the whole name or its initials:
// "Slot Hub Online" does not spell hub.
func wordsSpellLabel(folded, label string) bool {
	words := brandWords(folded)
	joined := strings.Join(words, "")
	if joined == label || (len([]rune(label)) >= 4 && strings.Contains(joined, label)) {
		return true
	}
	for _, word := range words {
		if len([]rune(word)) < 4 || genericBrandWords[word] {
			continue
		}
		if strings.Contains(label, word) || namematch.JaroWinkler(word, label) >= domainPersonalSimilarity {
			return true
		}
	}
	core := brandWords(NormalizeCompanyName(folded))
	significant := 0
	for _, word := range core {
		if !initialConnectives[word] {
			significant++
		}
	}
	return significant >= 2 && slices.Contains(initialsOf(core), label)
}

// brandFolds are the two spellings a name is compared in: accents dropped
// ("muller") and umlauts written out ("mueller"). Both write đ as d, which
// the shared fold keeps.
func brandFolds(name string) []string {
	return []string{
		strings.ReplaceAll(namematch.Fold(name), "đ", "d"),
		strings.ReplaceAll(namematch.Fold(namematch.Digraphs(name)), "đ", "d"),
	}
}

func brandWords(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// initialsOf lists every reading of a name's initials, taking each connective
// out, as its letter, or whole.
func initialsOf(words []string) []string {
	readings := []string{""}
	for _, word := range words {
		first := string([]rune(word)[0])
		next := make([]string, 0, len(readings)*3)
		for _, prefix := range readings {
			next = append(next, prefix+first)
			if initialConnectives[word] {
				next = append(next, prefix, prefix+word)
			}
		}
		readings = next
	}
	return readings
}

// compactNames are a company name with its legal form and spacing taken out, in
// both foldings: the shape a domain label has. "Example Analytics GmbH" reads
// exampleanalytics.
func compactNames(name string) []string {
	var out []string
	for _, folded := range brandFolds(name) {
		if key := strings.Join(brandWords(NormalizeCompanyName(folded)), ""); key != "" {
			out = append(out, key)
		}
	}
	return out
}

// SiteStatedNames lists the names a read says the site gives itself: its
// display name and its legal name.
func SiteStatedNames(fields []DeepReadField) []string {
	var out []string
	for _, f := range fields {
		if f.Field == fieldDisplayName || f.Field == fieldLegalName {
			out = append(out, f.Value)
		}
	}
	return out
}

func (in ResolveDomainTriageInput) siteSpeaksForDomain() bool {
	stated := append([]string{in.DossierName}, SiteStatedNames(in.Fields)...)
	return SiteSpeaksFor(stated, CompanyIdentity{Domains: []string{in.Domain}})
}

// withholdForeignSite holds a machine's company verdict whose site names a
// business that neither this domain nor a company here accounts for. That is
// what a taken-over domain looks like, so a human answers it. A near match to a
// company here still creates and files the pair, which already asks a human.
func withholdForeignSite(ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput, match CompanyMatch) (bool, error) {
	if in.decidedByAUser() || match.Decision == DecisionFuzzyReview || in.siteSpeaksForDomain() {
		return false, nil
	}
	return true, withholdDomainTx(ctx, tx, in.Domain, PendingForeignSite, in.Evidence, in.ReadID)
}

// SiteSpeaksForCompany asks SiteSpeaksFor against what the workspace holds for
// one company. It reads the company, so it takes the read gates.
func (s *Store) SiteSpeaksForCompany(ctx context.Context, companyID ids.CompanyID, stated []string) (bool, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionRead); err != nil {
		return false, err
	}
	var speaks bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, entityCompany, companyID.UUID); err != nil {
			return err
		}
		var known CompanyIdentity
		var displayName, legalName string
		if err := tx.QueryRow(ctx, `SELECT display_name, coalesce(legal_name, '') FROM company WHERE id = $1`,
			companyID).Scan(&displayName, &legalName); err != nil {
			return fmt.Errorf("contacts: reading the names a site read is checked against: %w", err)
		}
		known.Names = []string{displayName, legalName}
		domains, err := liveCompanyDomains(ctx, tx, companyID)
		if err != nil {
			return err
		}
		known.Domains = domains
		speaks = SiteSpeaksFor(stated, known)
		return nil
	})
	if err != nil {
		return false, err
	}
	return speaks, nil
}

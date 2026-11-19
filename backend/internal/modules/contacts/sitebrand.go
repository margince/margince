// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Whether a website speaks for the company it was read for. A domain can change
// hands, and a lapsed one may now serve a casino or a spam shop. That site's
// name, text, logo and staff describe another business. Every writer that lands
// a site read on a company, or names a new one after it, asks SiteSpeaksFor.

import (
	"context"
	"fmt"
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
		storedKey, statedKey := compactName(name), compactName(stated)
		if storedKey != "" && statedKey != "" &&
			(statedNameSpellsLabel(stated, storedKey) || statedNameSpellsLabel(name, statedKey)) {
			return true
		}
	}
	return false
}

// statedNameSpellsLabel reports whether a company name accounts for a domain
// label. "Kestner Consulting" spells kestnerconsulting, and "Marsh Advisory"
// spells rowanmarsh through its word "marsh". "Bayerische Motoren Werke AG"
// spells bmw through its initials.
func statedNameSpellsLabel(stated, label string) bool {
	label = namematch.Fold(strings.Join(strings.FieldsFunc(label, isNameSeparator), ""))
	words := brandWords(namematch.Fold(stated))
	if label == "" || strings.Contains(strings.Join(words, ""), label) {
		return true
	}
	for _, word := range words {
		// Four runes keeps a legal suffix ("AG", "Ltd") from matching by itself.
		if (len([]rune(word)) >= 4 && strings.Contains(label, word)) || namematch.JaroWinkler(word, label) >= domainPersonalSimilarity {
			return true
		}
	}
	core := brandWords(NormalizeCompanyName(stated))
	return len(core) >= 2 && initialsOf(core) == label
}

func brandWords(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func initialsOf(words []string) string {
	var out strings.Builder
	for _, word := range words {
		out.WriteRune([]rune(word)[0])
	}
	return out.String()
}

// compactName is a company name with its legal form and spacing taken out, the
// shape a domain label has: "Example Analytics GmbH" reads exampleanalytics.
func compactName(name string) string {
	return strings.Join(brandWords(NormalizeCompanyName(name)), "")
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
	return true, withholdDomainTx(ctx, tx, in.Domain, PendingUnevidenced, in.Evidence, in.ReadID)
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

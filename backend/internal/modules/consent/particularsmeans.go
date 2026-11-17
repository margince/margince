// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Whether a stated particular names each way of reaching someone that a pack
// asks for: a web page, an email address or a phone number.

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

var (
	emailInText = regexp.MustCompile(`[^\s@<>()\[\],;:]+@[^\s@<>()\[\],;:]+\.[\pL]{2,}`)
	// A scheme, a www. host, or a host with a dotted name such as zalo.me/acme.
	webInText = regexp.MustCompile(`(?i)(https?://\S+|\bwww\.\S+|\b[a-z0-9][a-z0-9-]*(\.[a-z0-9-]+)*\.[a-z]{2,}(/\S*)?)`)
	// Digits with the separators a written number uses. Slashes are left out,
	// so a date such as 16/11/2026 is not read as a number to call.
	phoneInText = regexp.MustCompile(`\+?\d[\d\s().-]{5,}\d`)
	isoDate     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// The length of a phone number in digits, the range values.E164Pattern allows.
// The pattern itself is not matched: free text writes a national number with
// a leading 0, spaces and brackets, which E.164 refuses.
const (
	phoneMinDigits = 8
	phoneMaxDigits = 15
)

// meansMissing answers which of the asked means text does not name, in the
// order they were asked.
func meansMissing(text string, means []messagingrules.ContactMeans) []messagingrules.ContactMeans {
	// Addresses are cut out first. The host of an email address is then not
	// read as a web page, and the digits of a link not as a number.
	withoutEmail := emailInText.ReplaceAllString(text, " ")
	withoutLinks := webInText.ReplaceAllString(withoutEmail, " ")
	var missing []messagingrules.ContactMeans
	for _, m := range means {
		var named bool
		switch m {
		case messagingrules.MeansEmail:
			named = emailInText.MatchString(text)
		case messagingrules.MeansWeb:
			named = webInText.MatchString(withoutEmail)
		case messagingrules.MeansPhone:
			named = namesPhone(withoutLinks)
		}
		if !named && !slices.Contains(missing, m) {
			missing = append(missing, m)
		}
	}
	return missing
}

func namesPhone(text string) bool {
	for _, candidate := range phoneInText.FindAllString(text, -1) {
		trimmed := strings.TrimSpace(candidate)
		if isoDate.MatchString(trimmed) {
			continue
		}
		digits := 0
		for _, r := range trimmed {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits >= phoneMinDigits && digits <= phoneMaxDigits {
			return true
		}
	}
	return false
}

// meansWords names the means for a field error an operator reads.
func meansWords(means []messagingrules.ContactMeans) string {
	words := make([]string, 0, len(means))
	for _, m := range means {
		words = append(words, meansWord(m))
	}
	return strings.Join(words, ", ")
}

func meansWord(m messagingrules.ContactMeans) string {
	switch m {
	case messagingrules.MeansWeb:
		return "a web page or social-network link"
	case messagingrules.MeansEmail:
		return "an email address"
	}
	return "a phone number"
}

// checkParticularsInForce refuses particulars that leave out a means a pack in
// force for this installation asks for. A field left empty is not checked
// here: the send refuses an advertising mail that owes it.
//
// The packs in force are those of the installation's country and of each live
// legal entity's country.
func (s *Store) checkParticularsInForce(ctx context.Context, p ControllerParticulars) error {
	inForce, err := s.rulesInForce(ctx)
	if err != nil {
		return err
	}
	versions := [][]particularField{particularFields(p, "")}
	for _, t := range p.Translations {
		versions = append(versions, particularFields(t.asParticulars(), t.Language))
	}
	for _, rules := range inForce {
		for _, d := range rules.Disclosures {
			if err := checkMeansStated(d, versions); err != nil {
				return err
			}
		}
	}
	return nil
}

// rulesInForce answers the rule set of each pack in force: the installation's
// country and each live legal entity's country. Both what settings refuse and
// what the settings form asks for are read off this one answer.
func (s *Store) rulesInForce(ctx context.Context) ([]messagingrules.Rules, error) {
	var inForce []messagingrules.Rules
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		codes, err := s.countriesInForceTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, code := range codes {
			if rules, _, found := applicableRules(code); found {
				inForce = append(inForce, rules)
			}
		}
		return nil
	})
	return inForce, err
}

// checkMeansStated answers the first stated field of d's kind that lacks one
// of d's means, as the field error that names what is missing.
func checkMeansStated(d messagingrules.Disclosure, versions [][]particularField) error {
	if len(d.Means) == 0 {
		return nil
	}
	for _, fields := range versions {
		for _, f := range fields {
			if f.kind != d.Kind || strings.TrimSpace(f.value) == "" {
				continue
			}
			if missing := meansMissing(f.value, d.Means); len(missing) > 0 {
				return &ValidationError{Field: f.name, Reason: fmt.Sprintf(
					"add %s: this text must name each of %s", meansWord(missing[0]), meansWords(d.Means))}
			}
		}
	}
	return nil
}

// countriesInForceTx is the installation's country and every live legal
// entity's country, each once.
func (s *Store) countriesInForceTx(ctx context.Context, tx pgx.Tx) ([]jurisdiction.Code, error) {
	installation, err := s.defaultEntityCountry(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT lower(country) FROM legal_entity
		 WHERE archived_at IS NULL AND coalesce(country, '') <> ''`)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the legal entities' countries: %w", err)
	}
	entityCountries, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("consent: reading the legal entities' countries: %w", err)
	}
	codes := []jurisdiction.Code{installation}
	for _, c := range entityCountries {
		if code := jurisdiction.Code(c); code != installation {
			codes = append(codes, code)
		}
	}
	return codes, nil
}

// particularsAsked answers what the packs in force ask of the particulars.
// That is the packs' disclosure languages, and the means an objection route
// names.
func (s *Store) particularsAsked(ctx context.Context) ([]string, []messagingrules.ContactMeans, error) {
	inForce, err := s.rulesInForce(ctx)
	if err != nil {
		return nil, nil, err
	}
	languages := []string{}
	var means []messagingrules.ContactMeans
	for _, rules := range inForce {
		if language := rules.DisclosureLanguage; language != "" && !slices.Contains(languages, language) {
			languages = append(languages, language)
		}
		for _, d := range rules.Disclosures {
			if d.Kind != messagingrules.ObjectionRoute {
				continue
			}
			for _, m := range d.Means {
				if !slices.Contains(means, m) {
					means = append(means, m)
				}
			}
		}
	}
	return languages, means, nil
}

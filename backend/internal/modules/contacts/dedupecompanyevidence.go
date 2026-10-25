// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A similar company name alone is weak evidence: "Kim Achterberg" and "Kim
// Fontaine" share a leading given name and score over the review bar. So the
// fuzzy tier files a name pair only when the two names are one brand. Any
// other name pair needs an address, a phone number or a current contact in
// common. A shared live domain is not asked here: uq_company_domain keeps it
// on one company, and the exact and subdomain lanes file nested ones.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// keepCorroborated keeps the name rivals a pair may be filed for. A subject
// not stored yet (a create) has nothing to share, so only same-brand rivals
// stay. The duplicate scan reads the new company again once it is stored.
func keepCorroborated(ctx context.Context, tx pgx.Tx, subject *ids.CompanyID,
	ranked []CompanyCandidateScore,
) ([]CompanyCandidateScore, error) {
	var unproven []ids.CompanyID
	for _, rival := range ranked {
		if !rival.SameBrand {
			unproven = append(unproven, rival.CompanyID)
		}
	}
	shared := map[ids.CompanyID]bool{}
	if subject != nil && len(unproven) > 0 {
		var err error
		if shared, err = companiesSharingEvidence(ctx, tx, *subject, unproven); err != nil {
			return nil, err
		}
	}
	kept := ranked[:0:0]
	for _, rival := range ranked {
		if rival.SameBrand || shared[rival.CompanyID] {
			kept = append(kept, rival)
		}
	}
	return kept, nil
}

// companyEvidence is what one company could share with another.
type companyEvidence struct {
	addressKey    string
	phones        []string
	sharesContact bool
}

// companiesSharingEvidence answers which rivals share an address, a phone
// number or a currently employed contact with the subject. It is one statement,
// over the primary key, uq_company_fact and the relationship indexes.
func companiesSharingEvidence(ctx context.Context, tx pgx.Tx, subject ids.CompanyID,
	rivals []ids.CompanyID,
) (map[ids.CompanyID]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.address_line1, c.address_line2, c.address_city, c.address_region,
		       c.address_postal_code, c.address_country,
		       coalesce((SELECT array_agg(f.value) FROM company_fact f
		                  WHERE f.company_id = c.id AND f.category = $3 AND f.field = $4), '{}'),
		       c.id <> $1 AND EXISTS (
		         SELECT 1 FROM relationship s
		           JOIN relationship r ON r.contact_id = s.contact_id
		           JOIN contact p ON p.id = s.contact_id AND p.archived_at IS NULL
		          WHERE s.company_id = $1 AND r.company_id = c.id
		            AND s.kind = 'employment' AND r.kind = 'employment'
		            AND s.archived_at IS NULL AND r.archived_at IS NULL
		            AND `+employment.IsCurrentSQL("s.ended_at")+`
		            AND `+employment.IsCurrentSQL("r.ended_at")+`)
		  FROM company c
		 WHERE c.id = $1 OR c.id = ANY($2)`, subject, rivals, factCategoryCompany, FactPhone)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading what similar companies share: %w", err)
	}
	defer rows.Close()
	evidence := map[ids.CompanyID]companyEvidence{}
	for rows.Next() {
		var id ids.CompanyID
		var line1, line2, city, region, postal, country *string
		var phones []string
		var held companyEvidence
		if err := rows.Scan(&id, &line1, &line2, &city, &region, &postal, &country,
			&phones, &held.sharesContact); err != nil {
			return nil, fmt.Errorf("contacts: reading what similar companies share: %w", err)
		}
		held.addressKey = streetAddressKey(line1, line2, city, region, postal, country)
		held.phones = comparablePhones(phones)
		evidence[id] = held
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading what similar companies share: %w", err)
	}
	own := evidence[subject]
	shared := make(map[ids.CompanyID]bool, len(rivals))
	for _, rival := range rivals {
		shared[rival] = own.shares(evidence[rival])
	}
	return shared, nil
}

// shares reports whether the rival holds one fact in common with this company.
func (own companyEvidence) shares(rival companyEvidence) bool {
	if rival.sharesContact || (own.addressKey != "" && own.addressKey == rival.addressKey) {
		return true
	}
	for _, phone := range own.phones {
		if slices.Contains(rival.phones, phone) {
			return true
		}
	}
	return false
}

// streetAddressKey is the geocoder's identity of an address (addressQuery),
// asked only of an address with a street line. Two companies in one city or
// one postal code have not said they sit at one door.
func streetAddressKey(line1, line2, city, region, postal, country *string) string {
	if line1 == nil || strings.TrimSpace(*line1) == "" {
		return ""
	}
	return addressHash(addressQuery(line1, line2, city, region, postal, country))
}

// comparablePhones is each stated number in E.164. A number with no country
// prefix cannot be told from the same digits in another country, so it is left
// out rather than guessed.
func comparablePhones(stated []string) []string {
	out := make([]string, 0, len(stated))
	for _, raw := range stated {
		if parsed, err := values.ParsePhone(raw); err == nil {
			out = append(out, parsed.String())
		}
	}
	return out
}

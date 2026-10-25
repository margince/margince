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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// keepCorroborated keeps the name rivals a pair may be filed for. A stored
// subject (ExcludeID) is compared on what it holds; a company not stored yet
// is compared on the address and numbers the candidate carries.
func keepCorroborated(ctx context.Context, tx pgx.Tx, c CompanyCandidate,
	ranked []CompanyCandidateScore,
) ([]CompanyCandidateScore, error) {
	var unproven []ids.CompanyID
	for _, rival := range ranked {
		if !rival.SameBrand {
			unproven = append(unproven, rival.CompanyID)
		}
	}
	shared := map[ids.CompanyID]bool{}
	if len(unproven) > 0 {
		var err error
		if shared, err = companiesSharingEvidence(ctx, tx, c, unproven); err != nil {
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

// candidateEvidence is what a company not stored yet brings to the comparison.
func candidateEvidence(c CompanyCandidate) companyEvidence {
	var own companyEvidence
	if c.Address != nil {
		own.addressKey = streetAddressKey(*c.Address)
	}
	own.phones = comparablePhones(c.Phones)
	return own
}

// companiesSharingEvidence answers which rivals share one fact with the
// subject. The fact is a street address, a number that is no switchboard, or a
// current contact every seat can see.
func companiesSharingEvidence(ctx context.Context, tx pgx.Tx, c CompanyCandidate,
	rivals []ids.CompanyID,
) (map[ids.CompanyID]bool, error) {
	evidence, err := readCompanyEvidence(ctx, tx, c.ExcludeID, rivals)
	if err != nil {
		return nil, err
	}
	own := candidateEvidence(c)
	if c.ExcludeID != nil {
		own = evidence[*c.ExcludeID]
	}
	shared := make(map[ids.CompanyID]bool, len(rivals))
	sharedNumbers := map[ids.CompanyID][]string{}
	var numbers []string
	for _, rival := range rivals {
		held := evidence[rival]
		if held.sharesContact || (own.addressKey != "" && own.addressKey == held.addressKey) {
			shared[rival] = true
			continue
		}
		for _, phone := range own.phones {
			if slices.Contains(held.phones, phone) {
				sharedNumbers[rival] = append(sharedNumbers[rival], phone)
				numbers = append(numbers, phone)
			}
		}
	}
	if len(numbers) == 0 {
		return shared, nil
	}
	switchboards, err := switchboardNumbers(ctx, tx, numbers, c.ExcludeID == nil)
	if err != nil {
		return nil, err
	}
	for rival, common := range sharedNumbers {
		shared[rival] = slices.ContainsFunc(common, func(phone string) bool { return !switchboards[phone] })
	}
	return shared, nil
}

// readCompanyEvidence reads the address and stated numbers of the rivals and
// of a stored subject, in one statement over the primary key and
// uq_company_fact. Each rival also answers whether a contact is employed at it
// and at the subject now, over the relationship company and contact indexes.
// Only a contact every seat can see counts, so a suggestion never discloses an
// employment kept private by its owner.
func readCompanyEvidence(ctx context.Context, tx pgx.Tx, subject *ids.CompanyID,
	rivals []ids.CompanyID,
) (map[ids.CompanyID]companyEvidence, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	subjectAt, rivalsAt := arg(subject), arg(rivals)
	categoryAt, fieldAt := arg(factCategoryCompany), arg(FactPhone)
	visible := auth.EverySeatVisible(entityContact, arg)("p")
	current := func(edge string) string {
		return employment.IsCurrentSQL(edge+".ended_at") + " AND " + employment.InPlaceAtSQL(edge, "now()")
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.address_line1, c.address_line2, c.address_city, c.address_region,
		       c.address_postal_code, c.address_country,
		       coalesce((SELECT array_agg(f.value) FROM company_fact f
		                  WHERE f.company_id = c.id AND f.category = $%[3]d AND f.field = $%[4]d), '{}'),
		       coalesce(c.id <> $%[1]d::uuid AND EXISTS (
		         SELECT 1 FROM relationship s
		           JOIN relationship r ON r.contact_id = s.contact_id
		           JOIN contact p ON p.id = s.contact_id AND p.archived_at IS NULL AND %[5]s
		          WHERE s.company_id = $%[1]d::uuid AND r.company_id = c.id
		            AND s.kind = 'employment' AND r.kind = 'employment'
		            AND s.archived_at IS NULL AND r.archived_at IS NULL
		            AND %[6]s AND %[7]s), false)
		  FROM company c
		 WHERE c.id = $%[1]d::uuid OR c.id = ANY($%[2]d)`,
		subjectAt, rivalsAt, categoryAt, fieldAt, visible, current("s"), current("r")), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading what similar companies share: %w", err)
	}
	defer rows.Close()
	evidence := map[ids.CompanyID]companyEvidence{}
	for rows.Next() {
		var id ids.CompanyID
		var address crmcontracts.Address
		var phones []string
		var held companyEvidence
		if err := rows.Scan(&id, &address.Line1, &address.Line2, &address.City, &address.Region,
			&address.PostalCode, &address.Country, &phones, &held.sharesContact); err != nil {
			return nil, fmt.Errorf("contacts: reading what similar companies share: %w", err)
		}
		held.addressKey = streetAddressKey(address)
		held.phones = comparablePhones(phones)
		evidence[id] = held
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading what similar companies share: %w", err)
	}
	return evidence, nil
}

// streetAddressKey is a postal address folded part by part, suite line
// included, so two suites at one street are two addresses. It is asked only of
// an address with a street line: one city or postal code is not one door.
func streetAddressKey(a crmcontracts.Address) string {
	if foldedNameKey(derefOr(a.Line1)) == "" {
		return ""
	}
	parts := []*string{a.Line1, a.Line2, a.PostalCode, a.City, a.Region, a.Country}
	folded := make([]string, 0, len(parts))
	for _, part := range parts {
		folded = append(folded, foldedNameKey(derefOr(part)))
	}
	return strings.Join(folded, "|")
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

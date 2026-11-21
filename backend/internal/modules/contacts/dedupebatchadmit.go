// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The candidate queries of the ladder return a stored record for scoring only
// when it passes their admission test, which runs in Postgres. A batch's held
// records are not stored, so the same test runs over their names as values.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// companyAdmitted answers whether fuzzyCompany's query returns one of held.
func companyAdmitted(ctx context.Context, inTx InTx, c NewCompany, held []heldCompany) (bool, error) {
	if len(held) == 0 {
		return false, nil
	}
	displays, legals := make([]string, 0, len(held)), make([]string, 0, len(held))
	for _, company := range held {
		displays, legals = append(displays, company.record.DisplayName), append(legals, company.record.LegalName)
	}
	args := []any{displays}
	displaysAt := "$" + strconv.Itoa(len(args))
	args = append(args, legals)
	legalsAt := "$" + strconv.Itoa(len(args))
	arms := companyAdmissionArms(&args, CompanyCandidate{DisplayName: c.DisplayName, LegalName: c.LegalName})
	if len(arms) == 0 {
		return false, nil
	}
	var admitted bool
	err := inTx(func(tx pgx.Tx) error {
		// The columns carry the stored names, so the arms read these rows as they
		// read the company table. An empty registered name is NULL, as stored.
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM (
			SELECT d AS display_name, NULLIF(l, '') AS legal_name
			  FROM unnest(`+displaysAt+`::text[], `+legalsAt+`::text[]) AS m(d, l)) AS company
			 WHERE `+strings.Join(arms, " OR ")+`)`, args...).Scan(&admitted)
	})
	if err != nil {
		return false, fmt.Errorf("contacts: asking the company query about a batch's companies: %w", err)
	}
	return admitted, nil
}

// contactAdmitted answers whether fuzzyContact's query returns one of names.
func contactAdmitted(ctx context.Context, inTx InTx, name string, names []string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	var admitted bool
	err := inTx(func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM unnest(@held::text[]) AS p(full_name)
			 WHERE `+contactNameAdmitsSQL("p.full_name", "@asked")+`)`,
			pgx.NamedArgs{"held": names, "asked": name}).Scan(&admitted)
	})
	if err != nil {
		return false, fmt.Errorf("contacts: asking the contact query about a batch's contacts: %w", err)
	}
	return admitted, nil
}

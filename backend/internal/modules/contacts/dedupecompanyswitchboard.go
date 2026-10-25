// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// switchboardHolders is how many live companies stating one number make it a
// switchboard: a shared reception, a call centre or a group's head office. Such
// a number says nothing about two of its holders being one company.
const switchboardHolders = 3

// switchboardNumbers answers which of these E.164 numbers are switchboards.
// unstored counts the candidate being created as one more holder.
//
// The digits-only LIKE only narrows the stated values, which are free text;
// ParsePhone decides which number each one is. Company facts carry no index on
// the number, so this runs only for numbers a similar-named pair already shares.
func switchboardNumbers(ctx context.Context, tx pgx.Tx, numbers []string, unstored bool) (map[string]bool, error) {
	patterns := make([]string, 0, len(numbers))
	for _, number := range numbers {
		patterns = append(patterns, "%"+strings.TrimPrefix(number, "+"))
	}
	rows, err := tx.Query(ctx, `
		SELECT f.company_id, f.value FROM company_fact f
		  JOIN company c ON c.id = f.company_id AND c.archived_at IS NULL
		 WHERE f.category = $1 AND f.field = $2
		   AND regexp_replace(f.value, '\D', '', 'g') LIKE ANY($3)`,
		factCategoryCompany, FactPhone, patterns)
	if err != nil {
		return nil, fmt.Errorf("contacts: counting the companies that state a number: %w", err)
	}
	defer rows.Close()
	holders := map[string]map[ids.CompanyID]bool{}
	for rows.Next() {
		var company ids.CompanyID
		var stated string
		if err := rows.Scan(&company, &stated); err != nil {
			return nil, fmt.Errorf("contacts: counting the companies that state a number: %w", err)
		}
		for _, number := range comparablePhones([]string{stated}) {
			if holders[number] == nil {
				holders[number] = map[ids.CompanyID]bool{}
			}
			holders[number][company] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: counting the companies that state a number: %w", err)
	}
	extra := 0
	if unstored {
		extra = 1
	}
	switchboards := make(map[string]bool, len(numbers))
	for _, number := range numbers {
		switchboards[number] = len(holders[number])+extra >= switchboardHolders
	}
	return switchboards, nil
}

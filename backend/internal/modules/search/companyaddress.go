// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// Only readable subjects contribute addresses; their company ids are planned once, independently of company rows.
func companyAddressMatch(ctx context.Context, alias string, whole int, arg func(any) int) (string, error) {
	var matches []string
	for _, kind := range []string{entityContact, entityLead} {
		branch, found := branchFor(kind)
		if !found {
			continue
		}
		scope, admitted, err := branchScope(ctx, branch, "subject", arg)
		if err != nil {
			return "", err
		}
		if !admitted {
			continue
		}
		email := branch.identifier
		prefix := storekit.EscapeLikeSQL(fmt.Sprintf("f_fold_apostrophes(lower(btrim($%d)))", whole))
		query := fmt.Sprintf(`SELECT domain.company_id FROM %s address
 JOIN %s subject ON subject.id=address.%s AND subject.archived_at IS NULL
 JOIN company_domain domain ON domain.archived_at IS NULL
 AND domain.domain_folded=address.%s_domain_folded
 WHERE address.archived_at IS NULL AND f_fold_apostrophes(lower(address.%s)) LIKE %s || '%%' AND %s`,
			email.Table, branch.table, email.FK, email.Column, email.Column, prefix,
			conjunction(branch.narrowing("subject"), scope))
		matches = append(matches, query)
	}
	if len(matches) == 0 {
		return sqlFalse, nil
	}
	return fmt.Sprintf("%s.id=ANY(ARRAY(%s))", alias, strings.Join(matches, " UNION ")), nil
}

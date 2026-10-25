// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"
	"strings"
	"testing"
)

func TestEveryHolderKindRendersTheSharedFileRules(t *testing.T) {
	t.Parallel()
	rules := []string{
		"at.archived_at IS NULL",
		"NOT at.bytes_withheld",
		fmt.Sprintf("< %d", inlineImageCeiling),
		"a.archived_at IS NULL",
		"at.entity_type = 'activity'",
	}
	for _, holder := range []string{holderDeal, holderContact, holderCompany} {
		sql := documentMembership(holder, 1)
		for _, rule := range rules {
			if !strings.Contains(sql, rule) {
				t.Errorf("%s membership lacks %q:\n%s", holder, rule, sql)
			}
		}
	}
}

func TestOnlyTheCompanyReadsMailFilesThroughTheEmploymentWalk(t *testing.T) {
	t.Parallel()
	walk := CompanyFiledActivityExists(1)
	if !strings.Contains(documentMembership(holderCompany, 1), walk) {
		t.Error("the company membership does not use the links walk without participants")
	}
	for _, holder := range []string{holderDeal, holderContact} {
		if !strings.Contains(documentMembership(holder, 1), "l."+holder+"_id = $1") {
			t.Errorf("%s membership does not walk the mail's %s link", holder, holder)
		}
	}
}

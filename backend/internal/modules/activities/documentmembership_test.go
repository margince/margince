// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestEveryHolderKindRendersTheSharedFileRules(t *testing.T) {
	t.Parallel()
	rules := []string{
		"at.archived_at IS NULL",
		"NOT at.bytes_withheld",
		storekit.ListedMailFileSQL("mf"),
		"a.archived_at IS NULL",
		"mf.entity_type = 'activity'",
		"UNION ALL",
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
	if !strings.Contains(documentMembership(holderDeal, 1), storekit.MessageOnDealSQL("a.id", "$1")) {
		t.Error("the deal membership does not walk the mail's deal link the deal room walks")
	}
	if !strings.Contains(documentMembership(holderContact, 1), "l.contact_id = $1") {
		t.Error("the contact membership does not walk the mail's contact link")
	}
}

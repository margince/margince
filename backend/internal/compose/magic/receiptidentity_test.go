// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReceiptIdentityFollowsOnlyTheNativeApprovalLinks(t *testing.T) {
	audit, approval := ids.NewV7(), ids.NewV7()
	for _, test := range []struct {
		name, entityType, action, evidence string
		linked                             bool
	}{
		{"stage", typeDeal, "advance_stage", fmt.Sprintf(`{"stage_approval_id":%q}`, approval), true},
		{"other action", typeDeal, "update", fmt.Sprintf(`{"stage_approval_id":%q}`, approval), false},
		{"released effect", typeCompany, "update", fmt.Sprintf(`{"released_by_approval_id":%q}`, approval), true},
		{"legacy", typeDeal, "advance_stage", `{}`, false},
		{"malformed", typeDeal, "advance_stage", `{"stage_approval_id":"invalid"}`, false},
		{"malformed release", typeCompany, "update", `{"released_by_approval_id":"invalid"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := entry{ID: audit, EntityType: test.entityType, Action: test.action, Evidence: []byte(test.evidence)}
			want := audit
			if test.linked {
				want = approval
			}
			if ids.UUID(receiptIdentity(e)) != want {
				t.Fatal("receipt identity crossed unrelated audit evidence")
			}
		})
	}
}

func TestGroupedMagicLineRetainsEveryReceiptIdentity(t *testing.T) {
	now := clocktest.Now(t)
	first := auditRow("link-reconcile", "", `{"cohort_linked":0,"cohort_promoted":1}`, "", now)
	second := auditRow("link-reconcile", "", `{"cohort_linked":0,"cohort_promoted":1}`, "", now)
	lines, _ := linesOf(unmasked(t), []entry{first, second}, nil, 100)
	if len(lines) != 1 || lines[0].ReceiptIds == nil || len(*lines[0].ReceiptIds) != 2 {
		t.Fatalf("grouped identities: %+v", lines)
	}
	for _, e := range []entry{first, second} {
		if !slices.Contains(*lines[0].ReceiptIds, receiptIdentity(e)) {
			t.Fatal("group lost one receipt identity")
		}
	}
}

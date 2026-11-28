// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A company link cannot widen a deal or project recap’s audience. Company-only
// recaps use the company grant and row scope instead.
func chatSummaryArm(p principal.Principal, alias string, arg func(any) int) string {
	if p.Type == principal.PrincipalSystem {
		return "TRUE"
	}
	granted := func(table string) bool {
		return (table == tableDeal || table == tableProject) &&
			Check{Object: table, Actions: []principal.Action{principal.ActionRead}}.HeldBy(p)
	}
	return fmt.Sprintf(`(%[1]s.kind <> 'note' OR coalesce(%[1]s.source_system, '') <> '%[2]s'
	   OR EXISTS (SELECT 1 FROM activity_link chat_link WHERE chat_link.activity_id = %[1]s.id AND %[3]s)
       OR (NOT EXISTS (SELECT 1 FROM activity_link origin_link WHERE origin_link.activity_id=%[1]s.id AND origin_link.entity_type IN ('deal','project'))
           AND EXISTS (SELECT 1 FROM activity_link company_link WHERE company_link.activity_id=%[1]s.id AND %[4]s)))`,
		alias, provenance.ChatSummarySource, linkTargetVisible(p, "chat_link", arg, granted),
		linkTargetVisible(p, "company_link", arg, func(table string) bool {
			return table == tableCompany && Check{Object: tableCompany, Actions: []principal.Action{principal.ActionRead}}.HeldBy(p)
		}))
}

// ActivityChatSummaryArm is chatSummaryArm for the caller, for a reader that
// composes the audience arm without the discover gate, such as the audit trail.
func ActivityChatSummaryArm(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	return chatSummaryArm(p, alias, arg), nil
}

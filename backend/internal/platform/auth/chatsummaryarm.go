// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// chatSummaryArm limits a chat summary note to seats that hold read on a record
// it is linked to, and that row scope reaches. Row scope opens every deal and
// project to every seat. The object grant is what keeps the note's names from a
// seat that may not open the record.
func chatSummaryArm(p principal.Principal, alias string, arg func(any) int) string {
	if p.Type == principal.PrincipalSystem {
		return "TRUE"
	}
	granted := func(table string) bool {
		return (table == tableDeal || table == tableProject) &&
			Check{Object: table, Actions: []principal.Action{principal.ActionRead}}.HeldBy(p)
	}
	return fmt.Sprintf(`(%[1]s.kind <> 'note' OR coalesce(%[1]s.source_system, '') <> '%[2]s'
	   OR EXISTS (SELECT 1 FROM activity_link chat_link WHERE chat_link.activity_id = %[1]s.id AND %[3]s))`,
		alias, provenance.ChatSummarySource, linkTargetVisible(p, "chat_link", arg, granted))
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

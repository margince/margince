// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Postgres plans every copy of the content gate a statement carries, and a
// pooled connection keeps that memory. So no export member may carry two, and
// the audit member's three content arms read one gated set.
func TestEveryExportMemberCarriesAtMostOneContentGate(t *testing.T) {
	read := principal.ObjectGrant{Read: true}
	rep := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		TeamIDs: []ids.UUID{ids.NewV7()},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": read, "contact": read, "company": read, "deal": read, "lead": read, "relationship": read,
			},
			RowScope: principal.RowScopeOwn,
		},
	})
	arg := func(any) int { return 1 }
	for caller, ctx := range map[string]context.Context{
		"rep": rep, "system": principal.SystemActing(context.Background(), "export-gate-test"),
	} {
		for _, m := range exportMembers {
			with, where, err := memberScope(ctx, m, arg)
			if err != nil {
				t.Fatalf("%s, %s: rendering its scope: %v", caller, m.table, err)
			}
			if got := auth.ContentGateCopies(with + where); got > 1 {
				t.Errorf("%s, %s carries %d content gates, want at most 1", caller, m.table, got)
			}
			if m.scope == scopeAudit && !strings.Contains(where, exportReadableActivity) {
				t.Errorf("%s: the audit member reads no gated set: %s", caller, where)
			}
		}
	}
	if _, _, err := memberScope(rep, exportMember{table: "unregistered", scope: 255}, arg); err == nil {
		t.Error("an unknown scope mode rendered a scope; it must refuse")
	}
	for _, m := range []exportMember{{table: "audit_log", scope: scopeAudit}, {table: "attachment", scope: scopeAttachment}} {
		if _, _, err := memberScope(context.Background(), m, arg); err == nil {
			t.Errorf("%s rendered a scope with no actor bound; it must refuse", m.table)
		}
	}
}

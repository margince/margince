// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// A chat summary note is readable only by seats holding read on its deal. The
// audit trail keeps the row, actor and time for everyone and withholds the
// image, as it does for any activity the reader may not read.

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func (e *auditBoundaryEnv) seedChatSummaryAudit(t *testing.T) ids.UUID {
	t.Helper()
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, seed := range []struct {
		statement string
		args      []any
	}{
		{`INSERT INTO pipeline (id, name) VALUES ($1, 'Chat')`, []any{pipeline}},
		{`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, name, status, pipeline_id, stage_id, source, captured_by)
		  VALUES ($1, 'Acme renewal', 'open', $2, $3, 'seed', 'system')`, []any{deal, pipeline, stage}},
	} {
		if _, err := e.owner.Exec(context.Background(), seed.statement, seed.args...); err != nil {
			t.Fatal(err)
		}
	}
	note := e.seedID(t, `INSERT INTO activity (id, kind, subject, audience, source, source_system, captured_by, occurred_at)
		VALUES ($1, 'note', 'Discount agreed', 'workspace', 'chat_links', $2, 'system', $3)`, provenance.ChatSummarySource, boundaryEarlier)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, note, deal); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, after, occurred_at)
		VALUES ($1, 'human', 'user:'||$2::text, 'create', 'activity', $3,
		        jsonb_build_object('kind', 'note', 'subject', 'Discount agreed'), $4)`,
		ids.NewV7(), e.other, note, boundaryEarlier); err != nil {
		t.Fatal(err)
	}
	return note
}

func (e *auditBoundaryEnv) withDealRead(read bool) context.Context {
	p, _ := principal.Actor(e.ctx)
	p.Permissions.Objects = maps.Clone(p.Permissions.Objects)
	p.Permissions.Objects["deal"] = principal.ObjectGrant{Read: read}
	return principal.WithActor(e.ctx, p)
}

func TestAChatSummaryAuditImageNeedsReadOnTheDeal(t *testing.T) {
	e := setupAuditBoundary(t)
	note := e.seedChatSummaryAudit(t)
	entityType, limit := "activity", 50
	read := func(ctx context.Context) []AuditEntry {
		page, err := ListAuditLog(ctx, e.db, AuditFilter{EntityType: &entityType, EntityID: &note, Limit: &limit})
		if err != nil {
			t.Fatalf("ListAuditLog: %v", err)
		}
		return page.Entries
	}
	for name, tc := range map[string]struct {
		ctx  context.Context
		seen bool
	}{"without deal read": {e.withDealRead(false), false}, "with deal read": {e.withDealRead(true), true}} {
		entries := read(tc.ctx)
		if len(entries) != 1 {
			t.Fatalf("%s: %d audit rows, want the row kept for the trail", name, len(entries))
		}
		if got := strings.Contains(string(entries[0].After), "Discount agreed"); got != tc.seen {
			t.Errorf("%s: image shows the summary text = %v, want %v: %s", name, got, tc.seen, entries[0].After)
		}
	}
}

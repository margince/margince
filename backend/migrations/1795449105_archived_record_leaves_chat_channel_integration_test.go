// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// chatLinkShape is one link as the correction finds it and as it must leave it.
type chatLinkShape struct {
	slug                          string
	deal, project                 bool
	dealArchived, projectArchived bool
	// want is "retired", "deal", "project" or "both": what the live link carries.
	want, audit string
}

func TestAnArchivedRecordLeavesItsChatLinkAndAnEmptyLinkIsRetired(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1795405500")); err != nil {
		t.Fatalf("up: %v", err)
	}
	shapes := []chatLinkShape{
		{slug: "deal-archived", deal: true, dealArchived: true, want: "retired", audit: "archive"},
		{slug: "project-archived", project: true, projectArchived: true, want: "retired", audit: "archive"},
		{slug: "shared-deal-archived", deal: true, project: true, dealArchived: true, want: "project", audit: "update"},
		{slug: "shared-project-archived", deal: true, project: true, projectArchived: true, want: "deal", audit: "update"},
		{slug: "shared-both-archived", deal: true, project: true, dealArchived: true, projectArchived: true, want: "retired", audit: "archive"},
		{slug: "live", deal: true, project: true, want: "both"},
	}
	seedChatLinks(t, conn, shapes)

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	for _, s := range shapes {
		got := seedValue(t, conn, `SELECT CASE WHEN archived_at IS NOT NULL THEN 'retired'
			WHEN deal_id IS NOT NULL AND project_id IS NOT NULL THEN 'both'
			WHEN deal_id IS NOT NULL THEN 'deal' ELSE 'project' END FROM chat_link WHERE channel_slug = $1`, s.slug)
		if got != s.want {
			t.Errorf("%s: link is %s, want %s", s.slug, got, s.want)
		}
		audits := seedValue(t, conn, `SELECT coalesce(string_agg(a.action, ','), '') FROM audit_log a
			JOIN chat_link l ON l.id = a.entity_id
			WHERE a.entity_type = 'chat_link' AND a.actor_id = 'migration' AND l.channel_slug = $1`, s.slug)
		if audits != s.audit {
			t.Errorf("%s: migration audit rows %q, want %q", s.slug, audits, s.audit)
		}
	}
	if got := seedValue(t, conn, `SELECT (before->>'deal_id' IS NOT NULL AND after->>'deal_id' IS NULL)::text FROM audit_log a
		JOIN chat_link l ON l.id = a.entity_id WHERE l.channel_slug = 'shared-deal-archived' AND a.actor_id = 'migration'`); got != "true" {
		t.Errorf("the audit image of a deal leaving its link does not show it leave: %s", got)
	}
}

// seedChatLinks writes each shape's records live and links them, then
// archives what the shape names, as an archive before the correction left it.
func seedChatLinks(t *testing.T, conn *pgx.Conn, shapes []chatLinkShape) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(context.Background(), sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	user := seedValue(t, conn, `INSERT INTO app_user (email, display_name) VALUES ('chat@link.test', 'Chat') RETURNING id::text`)
	pipeline := seedValue(t, conn, `INSERT INTO pipeline (name) VALUES ('Chat') RETURNING id::text`)
	stage := seedValue(t, conn, `INSERT INTO stage (pipeline_id, name, "position") VALUES ($1, 'Qualified', 1) RETURNING id::text`, pipeline)
	company := seedValue(t, conn, `INSERT INTO company (display_name, owner_id, source, captured_by) VALUES ('Acme', $1, 'seed', 'system') RETURNING id::text`, user)
	connection := seedValue(t, conn, `INSERT INTO channel_connection (provider, channel_id, channel_label, address, credential_ref, status, connected_by)
		VALUES ('dispact', 'team', 'Margince', 'https://chat.test', 'ref', 'connected', $1) RETURNING id::text`, user)
	for _, s := range shapes {
		var project, deal *string
		if s.project {
			id := seedValue(t, conn, `INSERT INTO project (name, company_id, owner_id, source, captured_by)
				VALUES ($1, $2, $3, 'manual', 'system') RETURNING id::text`, s.slug, company, user)
			exec(`INSERT INTO relationship (kind, project_id, company_id, role, source, captured_by)
				VALUES ('project_company', $1, $2, 'customer', 'manual', 'human:x')`, id, company)
			project = &id
		}
		if s.deal {
			id := seedValue(t, conn, `INSERT INTO deal (name, status, owner_id, pipeline_id, stage_id, project_id, company_id, source, captured_by)
				VALUES ($1, 'open', $2, $3, $4, $5::uuid, $6, 'manual', 'system') RETURNING id::text`, s.slug, user, pipeline, stage, project, company)
			deal = &id
		}
		exec(`INSERT INTO chat_link (connection_id, provider, channel_slug, channel_name, deal_id, project_id, linked_by)
			VALUES ($1, 'dispact', $2, $2, $3::uuid, $4::uuid, $5)`, connection, s.slug, deal, project, user)
		if s.dealArchived {
			exec(`UPDATE deal SET archived_at = now() WHERE id = $1`, *deal)
		}
		if s.projectArchived {
			exec(`UPDATE project SET archived_at = now() WHERE id = $1`, *project)
		}
	}
}

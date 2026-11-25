// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// member is a rep who may read, edit and archive every deal and project.
func (e *flushEnv) member() context.Context {
	grant := principal.ObjectGrant{Read: true, Update: true, Delete: true}
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"deal": grant, "project": grant},
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func (e *flushEnv) db() *database.DB {
	return database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
}

// archiveDeal archives the deal through the deals store and hands its
// deal.archived event to the consumer.
func (e *flushEnv) archiveDeal(t *testing.T, deal ids.UUID) {
	t.Helper()
	if _, err := deals.NewStore(e.db(), deals.Installation{}).ArchiveDeal(e.member(), ids.From[ids.DealKind](deal), nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	e.handleOutboxed(t, "deal.archived", deal)
}

// archiveProject archives the project through the projects store, with the
// company edges compose binds, and hands its project.archived event to the
// consumer.
func (e *flushEnv) archiveProject(t *testing.T, project ids.UUID) {
	t.Helper()
	store := projects.NewStore(e.db()).WithCompanyEdges(contacts.AttachCompanyToProjectTx,
		projects.CompaniesFrom(contacts.CompaniesOnProjectTx), contacts.FirstCustomersOnProjectsTx)
	if _, err := store.ArchiveProject(e.member(), ids.From[ids.ProjectKind](project), nil); err != nil {
		t.Fatalf("archiving the project: %v", err)
	}
	e.handleOutboxed(t, "project.archived", project)
}

func (e *flushEnv) handleOutboxed(t *testing.T, eventType string, entity ids.UUID) {
	t.Helper()
	var raw []byte
	if err := e.owner.QueryRow(context.Background(), `SELECT envelope FROM event_outbox
		WHERE envelope->>'type' = $1 AND envelope->'entity'->>'id' = $2 ORDER BY seq DESC LIMIT 1`,
		eventType, entity.String()).Scan(&raw); err != nil {
		t.Fatalf("reading the %s event: %v", eventType, err)
	}
	var env events.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding the %s event: %v", eventType, err)
	}
	if err := e.sync.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling %s: %v", eventType, err)
	}
}

// linkTo links a record to an existing channel as the member does.
func (e *flushEnv) linkTo(kind string, id ids.UUID, channel string) (chatlinks.State, error) {
	no := false
	return e.store.Link(e.member(), kind, id, chatlinks.LinkRequest{Mode: chatlinks.ModeExisting, Channel: channel, SummarizeHistory: &no})
}

func (e *flushEnv) seedDeal(t *testing.T, name string, project *ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, project_id, company_id, source, captured_by)
		SELECT $1, $2, 'open', d.owner_id, d.pipeline_id, d.stage_id, $3::uuid,
		       (SELECT r.company_id FROM relationship r WHERE r.kind = 'project_company' AND r.project_id = $3::uuid),
		       'manual', 'system'
		FROM deal d WHERE d.id = $4`, id, name, project, e.deal)
	return id
}

func (e *flushEnv) seedProject(t *testing.T, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	company := ids.NewV7()
	e.exec(t, `INSERT INTO company (id, display_name, owner_id, source, captured_by) VALUES ($1, $2, $3, 'seed', 'system')`, company, name, e.rep)
	e.exec(t, `INSERT INTO project (id, name, company_id, owner_id, source, captured_by) VALUES ($1, $2, $3, $4, 'manual', 'system')`,
		id, name, company, e.rep)
	e.exec(t, `INSERT INTO relationship (kind, project_id, company_id, role, source, captured_by)
		VALUES ('project_company', $1, $2, 'customer', 'manual', 'human:x')`, id, company)
	return id
}

func (e *flushEnv) linkState(t *testing.T, link ids.UUID) (retired bool, deal, project *ids.UUID) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(), `SELECT archived_at IS NOT NULL, deal_id, project_id FROM chat_link WHERE id = $1`,
		link).Scan(&retired, &deal, &project); err != nil {
		t.Fatal(err)
	}
	return retired, deal, project
}

func TestArchivingADealFreesItsChannelForAnotherDeal(t *testing.T) {
	e := setupFlush(t)
	e.handle(t, "deal.stage_changed", "deal", e.deal, `{"to_stage_name":"Proposal"}`)
	e.archiveDeal(t, e.deal)

	if retired, _, _ := e.linkState(t, e.link); !retired {
		t.Fatal("the archived deal still holds its channel")
	}
	var audited int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log
		WHERE entity_type = 'chat_link' AND entity_id = $1 AND action = 'archive' AND actor_id = 'system:chat_links'`,
		e.link).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("retire audit rows = %d, %v; want 1 by the chat engine", audited, err)
	}
	e.flush(t)
	if posts := postTexts(e.chat); len(posts) != 0 {
		t.Fatalf("the bot posted for an archived deal: %q", posts)
	}

	second := e.seedDeal(t, "Acme expansion", nil)
	if _, err := e.linkTo(chatlinks.RecordDeal, second, "acme"); err != nil {
		t.Fatalf("linking the freed channel to a second deal: %v", err)
	}
}

func TestArchivingAProjectFreesItsChannelForAnotherProject(t *testing.T) {
	e := setupFlush(t)
	e.chat.AddChannel("rollout")
	first, second := e.seedProject(t, "Rollout"), e.seedProject(t, "Rollout two")
	linked, err := e.linkTo(chatlinks.RecordProject, first, "rollout")
	if err != nil {
		t.Fatal(err)
	}
	e.archiveProject(t, first)

	if retired, _, _ := e.linkState(t, linked.Link.ID); !retired {
		t.Fatal("the archived project still holds its channel")
	}
	if _, err := e.linkTo(chatlinks.RecordProject, second, "rollout"); err != nil {
		t.Fatalf("linking the freed channel to a second project: %v", err)
	}
}

func TestAChannelADealSharesWithItsProjectStaysWithTheOneStillLive(t *testing.T) {
	e := setupFlush(t)
	e.chat.AddChannel("shared")
	project := e.seedProject(t, "Rollout")
	deal := e.seedDeal(t, "Rollout deal", &project)
	linked, err := e.linkTo(chatlinks.RecordDeal, deal, "shared")
	if err != nil || linked.Link.ProjectID == nil {
		t.Fatalf("link = %+v, %v; want the deal with its project", linked, err)
	}

	e.archiveDeal(t, deal)
	retired, keptDeal, keptProject := e.linkState(t, linked.Link.ID)
	if retired || keptDeal != nil || keptProject == nil || *keptProject != project {
		t.Fatalf("after the deal's archive: retired=%v deal=%v project=%v; want the project alone on the live link",
			retired, keptDeal, keptProject)
	}

	e.archiveProject(t, project)
	if retired, _, _ := e.linkState(t, linked.Link.ID); !retired {
		t.Fatal("a link whose records are all archived still holds its channel")
	}
}

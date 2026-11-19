// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A chat summary note is readable only by seats holding read on its deal or
// project, on every read path and even when it is also linked to a company.

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const chatSummaryWord = "Zebrafish"

func TestAChatSummaryNoteNeedsReadOnItsDealOnEveryReadPath(t *testing.T) {
	e := Setup(t)
	pipeline, stage, deal, note := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	company := e.SeedCompany(t, "Brandt Automotive", nil)
	e.WsExec(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Chat')`, pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.WsExec(t, `INSERT INTO deal (id, name, status, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Brandt renewal', 'open', $2, $3, 'seed', 'system')`, deal, pipeline, stage)
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, audience, source, source_system, captured_by)
		VALUES ($1, 'note', $2, $2, 'workspace', 'chat_links', $3, 'system')`, note, chatSummaryWord+" discount agreed", provenance.ChatSummarySource)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, note, deal)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, note, company)

	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	e.GrantRole(t, e.AdminUser, "admin")
	passport := e.SeedPassportFor(t, OwnerConn(t), "chat note agent", e.AdminUser)
	check := func(name string, perms principal.Permissions, seen bool) {
		agent := e.AgentOnPassport(t, e.AdminUser, nil, perms, ids.From[ids.PassportKind](passport))
		read, readErr := registry.Invoke(agent, "read_record", json.RawMessage(fmt.Sprintf(`{"record_type":"activity","id":%q}`, note)))
		list, listErr := registry.Invoke(agent, "list_records", json.RawMessage(fmt.Sprintf(`{"record_type":"activity","filters":{"entity_type":"company","entity_id":%q}}`, company)))
		if seen && (readErr != nil || listErr != nil) {
			t.Fatalf("%s: the tool calls failed: read %v, list %v", name, readErr, listErr)
		}
		ctx := e.As(e.AdminUser, nil, perms)
		hits, err := search.NewStore(e.DB()).Search(ctx, search.Input{Query: chatSummaryWord, Types: []string{"activity"}, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		entity, entityID := "company", company
		counted, err := activities.NewStore(e.DB()).CountActivities(ctx, activities.ListActivitiesInput{EntityType: &entity, EntityID: &entityID})
		if err != nil {
			t.Fatal(err)
		}
		for path, shown := range map[string]bool{
			"read_record":  strings.Contains(string(read), chatSummaryWord),
			"list_records": strings.Contains(string(list), chatSummaryWord),
			"search":       len(hits.Hits) > 0,
			"count":        counted > 0,
		} {
			if shown != seen {
				t.Errorf("%s: %s shows the chat note = %v, want %v", name, path, shown, seen)
			}
		}
	}
	check("a deal reader", AdminPerms, true)

	e.WsExec(t, `UPDATE role SET permissions = jsonb_set(permissions, '{objects,deal,read}', 'false') WHERE key = 'admin'`)
	check("without deal read", withoutRead(AdminPerms, "deal"), false)
}

func withoutRead(perms principal.Permissions, object string) principal.Permissions {
	out := perms
	out.Objects = maps.Clone(perms.Objects)
	grant := out.Objects[object]
	grant.Read = false
	out.Objects[object] = grant
	return out
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// adminFixtures fill the operator's calls: the release's data tasks, the home
// repair review and the mapping of old loss notes.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var adminFixtures = map[string]func(p *recordPool) map[string]any{
	"control_data_task": func(p *recordPool) map[string]any {
		task := p.take("data_task")
		return map[string]any{"request_body": map[string]any{
			"task_id": p.fact(task, "task_id"), "step": p.fact(task, "step"), "action": "estimate", "version": versionOf(p, task),
		}}
	},
	"activate_home_repair": func(p *recordPool) map[string]any {
		review := p.take("home_repair_review")
		var wire map[string]any
		if json.Unmarshal([]byte(p.fact(review, "review")), &wire) != nil {
			wire = map[string]any{}
		}
		return map[string]any{"request_body": map[string]any{"version": versionOf(p, review), "review": wire}}
	},
	"start_search_reindex": func(p *recordPool) map[string]any {
		p.take("idle_reindex")
		return map[string]any{"request_body": map[string]any{"force": true}}
	},
	"read_site_read_logo":              func(p *recordPool) map[string]any { return map[string]any{"readId": p.take("logo_site_read")} },
	"read_onboarding_company_proposal": func(p *recordPool) map[string]any { p.take("onboarding_with_read"); return map[string]any{} },
	"read_card_photo":                  fileOf("card.png", "image/png", string(sweepPNG())),
	"preview_ai_routing": func(*recordPool) map[string]any {
		var config map[string]any
		raw, err := json.Marshal(ai.FakeRoutingConfig())
		if err != nil || json.Unmarshal(raw, &config) != nil {
			config = map[string]any{}
		}
		return map[string]any{"request_body": config}
	},
	"retry_automation_run": byID("failed_automation_run"),
	"apply_legacy_loss_reasons": func(p *recordPool) map[string]any {
		mapping := p.take("loss_mapping")
		return map[string]any{"request_body": map[string]any{"mappings": []any{map[string]any{"id": mapping, "version": versionOf(p, mapping)}}}}
	},
}

// seedDataTasks lays down and runs the release's data tasks, as the migrate
// step does before an installation serves.
func seedDataTasks(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if _, err := compose.DataTaskExecute(t.Context(), e.Pool, dataTaskBatches); err != nil {
		t.Fatalf("running the data tasks: %v", err)
	}
}

// dataTaskBatches bounds the work of the release's data tasks.
const dataTaskBatches = 1000

// dataTaskRow is one row of the data task list.
type dataTaskRow struct {
	Status     string `json:"status"`
	ReviewKind string `json:"review_kind"`
	TaskID     string `json:"task_id"`
	Step       string `json:"step"`
	Version    int    `json:"version"`
}

// adminSeeders make the states the operator's calls act on.
func adminSeeders(s sweepSeed) map[string]func() string {
	dataTasks := func() []dataTaskRow {
		var list struct {
			Data []dataTaskRow `json:"data"`
		}
		s.call("GET", "/v1/admin/data-tasks", nil, nil, &list)
		return list.Data
	}
	noted := func(row dataTaskRow) string {
		id := ids.NewV7().String()
		s.pool.note(id, "task_id", row.TaskID)
		s.pool.note(id, "step", row.Step)
		s.pool.note(id, "version", strconv.Itoa(row.Version))
		return id
	}
	// A reviewed run another role activated is paused, as an administrator
	// pauses one. The next may then be activated, and no run rewrites records.
	pauseActivated := func() {
		for _, row := range dataTasks() {
			if row.Status == "pending" || row.Status == "running" {
				s.call("POST", "/v1/admin/data-tasks/control", integration.AnyMap{
					"task_id": row.TaskID, "step": row.Step, "action": "pause", "version": row.Version,
				}, nil, nil)
			}
		}
	}
	return map[string]func() string{
		"data_task": func() string {
			rows := dataTasks()
			if len(rows) == 0 {
				s.t.Fatal("the release seeds no data task, so control_data_task has nothing to act on")
			}
			return noted(rows[0])
		},
		"home_repair_review": func() string {
			pauseActivated()
			var review json.RawMessage
			s.call("POST", "/v1/admin/home-repair/preview", integration.AnyMap{"limit": 10}, nil, &review)
			for _, row := range dataTasks() {
				if row.ReviewKind == "home_attention" {
					id := noted(row)
					s.pool.note(id, "review", string(review))
					return id
				}
			}
			s.t.Fatal("no data task reviews home attention")
			return ""
		},
		// The fleet-wide reindex another role started runs to its end through
		// the worker, so the next start finds none running.
		"idle_reindex": func() string {
			worker, err := compose.EmbedReindexWorkerForTest(s.e.Pool, sweepEmbedder)
			if err != nil {
				s.t.Fatalf("building the reindex worker: %v", err)
			}
			for _, job := range importjobtest.Queued(s.t, s.e, compose.EmbedReindexArgs{}.Kind()) {
				importjobtest.WorkJob(s.t, s.e, worker, job, 0)
			}
			return ""
		},
		// A lead-routing run that failed on the lead's creation. No route leaves a
		// failed run, and the sweep's app runs no workflow consumer to fail one.
		"failed_automation_run": func() string {
			me := apptest.SessionUserID(s.t, s.e)
			var automation struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			}
			s.call("POST", "/v1/automations", integration.AnyMap{"key": "assign_lead_owner", "name": uniqueName(), "params": integration.AnyMap{"owners": []string{me}}}, nil, &automation)
			s.call("PATCH", "/v1/automations/"+automation.ID, integration.AnyMap{"status": "enabled"}, pinned(automation.Version), nil)
			lead := s.create["lead"]()
			event := s.ownerScalar(`SELECT id::text FROM event_outbox WHERE envelope->>'type' = 'lead.created' AND envelope->'entity'->>'id' = $1`, lead)
			return apptest.SeedWorkflowRun(s.t, s.e, apptest.WorkflowRun{
				Automation: automation.ID, Status: "failed", Trigger: ids.MustParse(event), At: clocktest.Now(s.t),
			})
		},
		// The wizard's state names a read of the company's site, which the
		// proposal is drawn from.
		"onboarding_with_read": func() string {
			read := finishedSiteRead(s, false)
			var state struct {
				Version int `json:"version"`
			}
			s.e.Call(s.t, "GET", "/v1/onboarding/state", nil, nil, &state)
			s.call("PUT", "/v1/onboarding/state", integration.AnyMap{
				"expected_version": state.Version, "step": "voice", "source_mode": "website", "website_url": "https://example.test/role-sweep",
				"site_read_id": read, "company_draft": integration.AnyMap{},
			}, nil, nil)
			return read
		},
		// A deal lost with a free-text note, the note mapped to a reason code.
		"loss_mapping": func() string {
			pauseActivated()
			lost := apptest.DefaultPipeline(s.t, s.e).Lost
			note := "Sweep " + ids.NewV7().String()
			s.call("POST", "/v1/deals/"+s.create["deal"]()+"/advance", integration.AnyMap{"to_stage_id": lost, "lost_reason": note}, nil, nil)
			var legacy struct {
				Data []struct {
					TextHash string `json:"text_hash"`
					Text     string `json:"text"`
				} `json:"data"`
			}
			s.call("GET", "/v1/loss-reasons/legacy", nil, nil, &legacy)
			for _, row := range legacy.Data {
				if row.Text == note {
					var mapping struct {
						ID      string `json:"id"`
						Version int    `json:"version"`
					}
					s.call("PUT", "/v1/loss-reasons/legacy/"+row.TextHash, integration.AnyMap{"reason_key": "price"}, nil, &mapping)
					s.pool.note(mapping.ID, "version", strconv.Itoa(mapping.Version))
					return mapping.ID
				}
			}
			s.t.Fatalf("the lost deal's note %q is not listed as legacy", note)
			return ""
		},
	}
}

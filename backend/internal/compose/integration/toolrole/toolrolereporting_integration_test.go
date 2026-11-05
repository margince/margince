// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"context"
	"encoding/json"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// sweepSelection is a report selection every seeded role may save.
func sweepSelection() map[string]any {
	return map[string]any{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []any{"bookings_won"}}
}

// targetMonths hands each target the sweep sets its own month, so no two
// collide on one metric, scope and period.
var targetMonths atomic.Int32

func targetInput(p *recordPool, months int32) map[string]any {
	start := time.Date(p.sweepStart.Year(), p.sweepStart.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, int(months), 0)
	return map[string]any{
		"metric": "meetings_held", "scope": map[string]any{"kind": "workspace"}, "period_kind": "month",
		"period_start": start.Format(time.DateOnly), "value": 40, "reason": "The plan for the month.",
	}
}

// seededVersion is the version a seed read its record at. Counting mode answers a
// placeholder, which reads as no version.
func seededVersion(p *recordPool, id string) int {
	version, err := strconv.Atoi(p.fact(id, "version"))
	if err != nil {
		return 0
	}
	return version
}

// reportingFixtures fill the report, target, initiative and input check calls.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var reportingFixtures = map[string]func(p *recordPool) map[string]any{
	"create_reporting_report": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"name": uniqueName(), "audience": "private", "selection": sweepSelection()}}
	},
	"update_reporting_report": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("report"), "if_version": 1, "request_body": map[string]any{
			"name": uniqueName(), "audience": "private", "selection": sweepSelection(),
		}}
	},
	"restore_reporting_report_revision": func(p *recordPool) map[string]any {
		report := p.take("revised_report")
		return map[string]any{"id": report, "revision": "1", "if_version": seededVersion(p, report)}
	},
	"archive_reporting_report":        byID("report"),
	"list_reporting_report_revisions": byID("report"),
	"freeze_reporting_edition":        byID("editioned_report"),
	"create_reporting_schedule": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("editioned_report"), "request_body": map[string]any{
			"report_revision": 1, "frequency": "monthly", "day": 28, "local_time": "09:00", "enabled": true,
		}}
	},
	"update_reporting_schedule": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("report_schedule"), "if_version": 1, "request_body": map[string]any{
			"report_revision": 1, "frequency": "monthly", "day": 28, "local_time": "10:30", "enabled": true,
		}}
	},
	"retry_reporting_execution": byID("failed_execution"),
	"publish_reporting_report_draft": func(p *recordPool) map[string]any {
		report := p.take("drafted_report")
		return map[string]any{"id": report, "if_version": seededVersion(p, report)}
	},
	"discard_reporting_report_draft": func(p *recordPool) map[string]any {
		report := p.take("drafted_report")
		return map[string]any{"id": report, "if_version": seededVersion(p, report)}
	},
	"create_reporting_target": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": targetInput(p, 1+targetMonths.Add(1))}
	},
	"update_reporting_target": func(p *recordPool) map[string]any {
		target := p.take("sales_target")
		months, err := strconv.Atoi(p.fact(target, "months"))
		if err != nil {
			months = 0
		}
		input := targetInput(p, int32(months))
		input["value"], input["reason"] = 45, "More capacity."
		return map[string]any{"id": target, "if_version": 1, "request_body": input}
	},
	"publish_reporting_framework": func(p *recordPool) map[string]any {
		framework := p.take("framework")
		// Counting mode answers a placeholder, which is no definition.
		definition := map[string]any{}
		if err := json.Unmarshal([]byte(p.fact(framework, "definition")), &definition); err != nil {
			definition = map[string]any{}
		}
		definition["template"], definition["reason"] = "sdr", "Use agreed SDR outcomes."
		return map[string]any{"if_version": seededVersion(p, framework), "request_body": definition}
	},
	"launch_sales_initiative": func(p *recordPool) map[string]any {
		initiative := p.take("staffed_initiative")
		return map[string]any{"id": initiative, "if_version": seededVersion(p, initiative)}
	},
	"record_sales_decision": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{
			"decision": "Offer the pilot at cost.", "deal_id": p.take("deal"), "owner_id": p.take("me"),
			"review_on": p.sweepStart.AddDate(0, 1, 0).Format(time.DateOnly),
		}}
	},
	"record_sales_decision_outcome": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("sales_decision"), "request_body": map[string]any{"result": "They took the pilot."}}
	},
	"revoke_forecast_share": byID("forecast_share_id"),
	"wait_input_check": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("deal_finding"), "request_body": map[string]any{"until": p.sweepStart.AddDate(0, 0, 30).Format(time.DateOnly)}}
	},
	"close_lost_input_check": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("deal_finding"), "request_body": map[string]any{"lost_reason": "They chose a rival."}}
	},
	"resolve_input_check": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("next_step_finding"), "request_body": map[string]any{"outcome": "value_correct", "reason": "Checked with the buyer."}}
	},
	"explain_analytics_cell": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"query": map[string]any{
			"entity": "activities-by-kind", "group_by": []any{"kind"}, "measures": []any{map[string]any{"fn": "count", "as": "n"}},
		}, "group": []any{"task"}}}
	},
	"explain_report_run_cell": func(p *recordPool) map[string]any {
		return map[string]any{"run_id": p.take("report_run"), "request_body": map[string]any{"group": []any{"task"}}}
	},
	"explain_reporting_question_card": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("question_report"), "card_id": "q1", "request_body": map[string]any{"group": []any{p.take("company")}}}
	},
	"assess_sales_review": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"view": "sales", "selection": sweepSelection()}}
	},
	"preview_filter": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"resource": "contact", "filter": map[string]any{
			"field": "created_at", "op": "gte", "value": "2000-01-01",
		}}}
	},
	"propose_filter": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"resource": "contact", "text": "contacts named Role Sweep"}}
	},
	"propose_report_setup": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"site": "initiative_setup", "description": "Win back the spring fair."}}
	},
}

// reportingSeeders make the reports, targets, initiatives and findings above.
func reportingSeeders(s sweepSeed) map[string]func() string {
	report := func() string {
		return s.post("/v1/analytics/reports", integration.AnyMap{"name": uniqueName(), "audience": "private", "selection": sweepSelection()})
	}
	versioned := func(id, path string) string {
		var row struct {
			Version int64 `json:"version"`
		}
		s.call("GET", path, nil, nil, &row)
		s.pool.note(id, "version", strconv.FormatInt(row.Version, 10))
		return id
	}
	// An edition keeps its record only under a retention policy for it.
	editionKept := func() {
		s.ownerExec(`INSERT INTO retention_policy (object_type, retain_days, action, enabled)
			SELECT 'report_edition', 730, 'erase', true WHERE NOT EXISTS (SELECT 1 FROM retention_policy WHERE object_type = 'report_edition')`)
	}
	finding := func(kind string) string {
		deal := s.create["deal"]()
		return s.ownerScalar(`INSERT INTO assurance_exception (logical_key, type, subject_kind, subject_id, severity, owner_id, captured_by)
			VALUES ($1, $2, 'deal', $3, 'medium', $4, 'system') RETURNING id::text`,
			kind+":"+deal, kind, deal, apptest.SessionUserID(s.t, s.e))
	}
	return map[string]func() string{
		"revised_report": func() string {
			id := report()
			s.call("PATCH", "/v1/analytics/reports/"+id, integration.AnyMap{"name": uniqueName(), "audience": "private", "selection": sweepSelection()}, pinned(1), nil)
			return versioned(id, "/v1/analytics/reports/"+id)
		},
		"editioned_report": func() string {
			editionKept()
			return report()
		},
		"report_schedule": func() string {
			editionKept()
			return s.post("/v1/analytics/reports/"+report()+"/schedules", integration.AnyMap{
				"report_revision": 1, "frequency": "monthly", "day": 28, "local_time": "09:00", "enabled": true,
			})
		},
		"failed_execution": func() string {
			editionKept()
			var edition struct {
				ID string `json:"id"`
			}
			s.call("POST", "/v1/analytics/reports/"+report()+"/editions", nil, map[string]string{"Idempotency-Key": ids.NewV7().String()}, &edition)
			execution := edition.ID
			s.ownerExec(`UPDATE report_execution SET status = 'failed' WHERE id = $1`, execution)
			return execution
		},
		"drafted_report": func() string {
			id := report()
			s.call("PUT", "/v1/analytics/reports/"+id+"/draft", integration.AnyMap{"selection": sweepSelection()}, pinned(1), nil)
			return versioned(id, "/v1/analytics/reports/"+id)
		},
		"sales_target": func() string {
			months := 100 + targetMonths.Add(1)
			id := s.post("/v1/analytics/targets", targetInput(s.pool, months))
			s.pool.note(id, "months", strconv.Itoa(int(months)))
			return id
		},
		"framework": func() string {
			var framework struct {
				Version    int64           `json:"version"`
				Definition json.RawMessage `json:"definition"`
			}
			s.call("GET", "/v1/analytics/framework", nil, nil, &framework)
			s.pool.note("framework", "version", strconv.FormatInt(framework.Version, 10))
			s.pool.note("framework", "definition", string(framework.Definition))
			return "framework"
		},
		"staffed_initiative": func() string {
			id := s.create["initiative"]()
			s.call("PUT", "/v1/analytics/initiatives/"+id+"/members", integration.AnyMap{
				"picked": []integration.AnyMap{{"record_type": "lead", "record_id": s.create["lead"]()}}, "excluded": []integration.AnyMap{},
			}, nil, nil)
			return versioned(id, "/v1/analytics/initiatives/"+id)
		},
		"sales_decision": func() string {
			return s.post("/v1/analytics/decisions", integration.AnyMap{
				"decision": "Offer the pilot at cost.", "deal_id": s.create["deal"](), "owner_id": apptest.SessionUserID(s.t, s.e),
				"review_on": s.pool.sweepStart.AddDate(0, 1, 0).Format(time.DateOnly),
			})
		},
		"forecast_share_id": func() string {
			return s.post("/v1/forecast/shares", integration.AnyMap{"kind": "live", "target": "forecast"})
		},
		"deal_finding":      func() string { return finding("deal_viability") },
		"next_step_finding": func() string { return finding("no_next_step") },
	}
}

// sweepLane answers a model call with one fixed reply, for the proposals the
// sweep asks a model for.
type sweepLane struct{ reply string }

func (l sweepLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.reply}, nil
}

const (
	sweepFilterReply = `{"groups":[{"join":"and","clauses":[{"phrase":"named Role Sweep","field":"full_name","op":"eq",` +
		`"text":"Role Sweep","number":null,"flag":null,"list":null,"days_ago":null}]}],"join":"and","unsupported":[]}`
	sweepSetupReply = `{"name":"Spring fair","filters":[],"metrics":[],"charts":[],"objective":null,"clarifications":[],"unsupported":[]}`
)

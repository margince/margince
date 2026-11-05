// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
)

// projectFixtures fill the project, commission and input check calls.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var projectFixtures = map[string]func(p *recordPool) map[string]any{
	"correct_project_health_assessment": func(p *recordPool) map[string]any {
		assessment := p.take("project_assessment")
		return map[string]any{"id": p.fact(assessment, "project"), "assessment_id": assessment, "request_body": map[string]any{
			"state": "on_track", "note": "Back on plan.",
		}}
	},
	"transfer_project_ownership": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"from_owner_id": p.take("me"), "to_owner_id": p.take("teammate")}}
	},
	"decide_commission_entry": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("commission_entry"), "request_body": map[string]any{"decision": "approve"}}
	},
	"start_forecast_assurance_run": func(p *recordPool) map[string]any {
		p.take("idle_input_check")
		return map[string]any{}
	},
}

// projectSeeders make a project's health reading, a commission to decide and a
// workspace with no input check queued.
func projectSeeders(s sweepSeed) map[string]func() string {
	return map[string]func() string{
		"project_assessment": func() string {
			project := s.create["project"]()
			assessment := s.post("/v1/projects/"+project+"/health-assessments", integration.AnyMap{"state": "at_risk", "note": "Slipped."})
			s.pool.note(assessment, "project", project)
			return assessment
		},
		// A won partner deal accrues its commission in the ledger's own job.
		"commission_entry": func() string {
			return s.ownerScalar(`INSERT INTO commission_entry (id, deal_id, partner_company_id, attribution_at_accrual, rate_bps,
				basis_amount_minor, currency, amount_minor, captured_by)
				VALUES (uuidv7(), $1, $2, 'sourced', 1500, 100000, 'EUR', 15000, 'system') RETURNING id::text`,
				s.create["deal"](), s.create["company"]())
		},
		// The insert-only queue never works a run, so the one a call queued
		// would keep every later start waiting on it.
		"idle_input_check": func() string {
			s.ownerExec(`DELETE FROM river_job WHERE kind = $1 AND state IN ('available', 'scheduled', 'pending', 'retryable')`,
				compose.AssuranceRunArgs{}.Kind())
			return "idle"
		},
	}
}

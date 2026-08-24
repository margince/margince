// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package deals owns the deal aggregate and its pipeline scaffolding —
// creation (born open, never onto a terminal stage), keyset listing,
// optimistic updates, stage advancement with the won/lost semantics and
// FX freezing (formulas-and-rules), archive, and the per-workspace
// default-pipeline seed injected into identity's bootstrap at the
// composition root — as store + contract mapping + transport handlers +
// the deals slice of the datasource provider, flat per ADR-0054 §3.
//
// Tables owned: deal, deal_stage_history, deal_forecast_history, pipeline,
// stage, fx_rate,
// product, offer, offer_line_item, offer_template (the E03.16-.20 offer
// engine: rate-card products, versioned deal-bound offers with derived money
// totals).
//
// The project moved OUT of this module into modules/projects, superseding
// ADR-0073 — see that package's doc.go for the reasoning and for the two
// project-facing things that stayed here: the money a project's deals add up to,
// which is a deal read priced by this module's installation seam, and the
// deal↔project company rule, which faults a deal's own field.
//
// The two histories are separate on purpose. deal_stage_history answers what a
// deal looked like when it entered a stage, and readers outside this module
// count its rows as movements; deal_forecast_history records an amount or close
// date that moved WITHOUT a stage change (see forecasthistory.go).
//
// Imports shared + platform + the generated contract only; never a
// sibling module. Every write rides storekit's audit+outbox shape and
// every entry point is gated by platform/auth.
package deals

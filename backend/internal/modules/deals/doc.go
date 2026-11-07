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
// Tables owned: docs/reference/entity-model/deals.md.
//
// close_date_run and close_date_run_member hold the nightly close-date pass
// and the eligible set frozen at its start. So a pass resumes where it
// stopped, and can say how much of that set it covered.
//
// deal_correction is one correction's lifecycle, so it can be named, taken
// back, and not re-applied afterwards.
//
// deal_acquisition_source is the administered business channels a deal is
// attributed to: the opportunity's origin, distinct from the lead
// vocabulary's record provenance.
//
// deal_loss_reason is the administered list a lost deal names its reason
// from. deal_loss_reason_mapping is an administrator's reviewed code for one
// legacy note, keyed by the note's hash.
//
// deal_risk_day and deal_risk_verdict are each day's first risk pass and the
// deals it judged material and at risk. compose's hourly pass writes them,
// because the judgement cannot be recomputed afterwards.
//
// deal_suggestion and deal_suggestion_evidence are Deal Scout's proposals to
// open a deal and the evidence each cites. compose's hourly pass writes them
// through RecordSuggestionTx, and a rep decides them here.
//
// legal_review is a deal's request for legal's approval, the pack it was
// asked on, and legal's answer.
//
// The project moved OUT of this module into modules/projects, superseding
// ADR-0073 — see that package's doc.go for the reasoning and for the two
// project-facing things that stayed here: the money a project's deals add up to,
// which is a deal read priced by this module's installation seam, and the
// deal↔project company rule, which faults a deal's own field.
//
// deal_stage_history answers what a deal looked like when it entered a stage,
// and readers outside this module count its rows as movements. An amount or
// close date that moved without a stage change is in audit_log, with before
// and after values and the actor.
//
// Imports shared + platform + the generated contract only; never a
// sibling module. Every write rides storekit's audit+outbox shape and
// every entry point is gated by platform/auth.
package deals

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package projects owns the project aggregate — the body of work a deal is
// about — as store + contract mapping + transport handlers + the projects slice
// of the datasource provider, flat per ADR-0054 §3.
//
// It carries creation with its key, keyset listing, optimistic updates, the
// phase ladder with its history, ownership transfer, archive, the project
// surfaces a company and a contact page read, and the quiet-project predicate
// the signal scan and the reports share.
//
// Tables owned: project, project_phase_history.
// A project's stakeholders are `relationship` rows of kind project_stakeholder,
// and that table is people's.
//
// SUPERSEDES ADR-0073, which placed the project inside the deals bounded
// context on the reasoning that a project is what a deal turns into. That held
// while a project belonged to one company and hung off one deal. It stopped
// holding once the project became a record a company, a contact and a deal each
// link to, read from three surfaces and written by its own phase ladder: the
// deals module was carrying a second aggregate, and the two grew helpers that
// looked shared but were only adjacent — a project's list once sorted by a
// constant named for an offer template's name column, and the reasoning for
// both records had to be held at once to read either.
//
// What did NOT move, and why: the money a project's deals add up to
// (deals.ProjectDealTotalsTx, deals.OpenDealBaseValueSQL) is read from the deal
// table under the caller's DEAL row scope and priced by the deals module's
// installation seam, so it stays there and this module consumes it through a
// port compose injects. The deal↔project company rule
// (deals.DealProjectOrgMismatchError) likewise faults a deal's field on a deal
// write.
//
// The edges the other direction are ports too: deals asks this module whether a
// project may be attached (EnsureAttachable) and to advance a won deal's project
// into delivery (StartDeliveryForWonDeal), both inside the caller's own
// transaction so the answer cannot go stale and the two rows commit together.
// Compose binds them (compose/installseam) — a module never imports a sibling.
//
// Imports shared + platform + the generated contract only. Every write rides
// storekit's audit+outbox shape and every entry point is gated by platform/auth.
package projects

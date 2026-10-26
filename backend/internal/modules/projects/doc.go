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
// Tables owned: docs/reference/entity-model/projects.md.
// A project's stakeholders are `relationship` rows of kind project_stakeholder,
// and that table is contacts's.
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
// Deal valuation stays with the deals module and is injected through a port.
// The deal-to-project company constraint is also checked on the deal write.
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

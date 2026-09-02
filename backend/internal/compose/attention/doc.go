// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package attention assembles the one surface a rep opens to answer "what
// needs me?".
//
// Five producers already ask a rep for something, and before this they asked
// from four screens with four shapes: staged approvals in a queue, duplicate
// pairs in a second one, tasks in a third, the ranked Morning-Brief deals and
// the overnight digest on a fourth. Each was reasonable alone. Together they
// meant the rep's actual question had no single answer, and the cheapest way
// to answer it was to visit every screen and hold the total in their head.
//
// It lives in compose because it spans approvals, people, activities and the
// brief engine — the composition layer's charter — and it owns no table of its
// own. Nothing here is a second copy of a lifecycle: every lane is a READ
// through the owning module's own entry point, and every verb the client
// offers routes back to that owner's endpoint. This feed adds no decision
// authority, exactly as the per-account approvals panel adds none.
//
// Two rules shape it.
//
// The tier is derived from what an item COSTS, not from who raised it. An
// approval to send an email and a duplicate merge both land in needs_you
// because both are irreversible and only a person may choose; a task lands in
// planned because the choosing already happened; a receipt lands in
// done_for_you because the work is finished and the rep is owed the fact
// rather than a question. Sorting by producer instead would put a merge next
// to a reminder and leave the reader to work out which one matters.
//
// WHAT IS DELIBERATELY NOT A LANE, so an omission is not read as an oversight.
//
// review_commitments is an MCP tool, and the briefing plan listed it as a
// Worklist lane to build. It was not built, on purpose. Its seam reads open
// tasks through activities.ListOpenTasks and its result is keyed by task id —
// which is the same population `planned` already shows, plus the undated ones
// `planned` excludes on purpose. A lane for it would put the same rows on the
// same screen under a second heading, which is the duplication this package
// exists to end, and it would make three commitment surfaces of two: the
// claims lane, the task lane, and it.
//
// What would earn one is a different QUESTION rather than a second rendering of
// this one — the unassigned open promises, say, which is the state that the
// tool's own result type calls out and which `planned` cannot show. That lane
// deserves its own name rather than the plan's.
//
// The tool itself is untouched by any of this and still answers.
//
// A lane the caller may not read is OMITTED and named, never returned empty.
// "You may not see this" and "there is none" are different answers, and a
// surface whose whole promise is "this is your day" must not report a clear
// day it cannot actually see. Counts obey the same rule as their lane: a
// number a caller cannot page to would report the existence of records they
// may not read, which is the leak the dedupe queue's both-sides-visible clause
// exists to prevent.
package attention

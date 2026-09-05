// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package magic answers what the machinery did, needs, could not finish, and is
// watching.
//
// WHY IT EXISTS. Automation writes constantly — the lead ladder, close-date
// hygiene, capture linking, enrichment, follow-up proposals — and a rep saw
// almost none of it. The work landed in audit_log and in the records themselves,
// so the product read as either idle or spooky: changes appeared with no author
// anybody could name. This is the receipt.
//
// A RECEIPT AND A ROUTER, NEVER A SECOND INBOX. Nothing is answered from here. A
// pending approval is decided where approvals are decided; an undo calls the
// record's own restore route. The lanes report, and the surfaces that own each
// verb keep it. A second place to answer a decision is a second place for the
// two answers to disagree.
//
// THE SCOPING PROBLEM, which is the whole of the security work in this package.
// audit_log carries no workspace_id — migration 1787320004 dropped the tenant
// column from the append-only ledgers, and its own comment says no table in the
// schema carries one after it. So an audit row cannot be scoped BY ITSELF. Every
// row is placed by joining the table its entity_type names, under that table's
// own gate:
//
//   - the five owner-scoped records (deal, organization, person, lead, project)
//     through auth.ScopeClauseFor, which is the row-visibility predicate every
//     other read of them uses.
//   - activity through auth.ActivityContentClause, because an activity's
//     audience is not an owner question and a row-scope clause would miss it.
//   - approval through the approvals service, which holds its own reader rule.
//
// AN ENTITY TYPE THIS BUILD CANNOT PLACE IS NOT SHOWN. It is counted in
// not_shown instead. Serving a row this read cannot scope would be serving a row
// it cannot prove the reader may see, and the failure would be silent: the row
// looks like every other row.
//
// AND A MACHINE WRITE IS NOT AUTOMATICALLY MAGIC. actor_type tells you a machine
// acted; it does not tell you the action means anything to a customer. A
// maintenance sweep and a projection refresh are machine writes, and folding
// them in would turn internal churn into apparent value. The admitted actions
// are a closed list with a sentence, a consequence and an undo policy each;
// everything else is counted in not_shown.
package magic

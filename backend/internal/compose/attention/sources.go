// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The words a queue row carries for WHICH LANE produced it, and which subject
// it names.
//
// Named rather than written as literals at each site, because the readers are
// spread: the classifier sets them, the scope filters ask about them, the
// bounds table and the category map read them. A misspelt literal in any of
// those fails silently — it matches nothing, drops nothing, and reads green.

// sourceWaiting names the who-is-waiting producer. A named constant rather
// than the literal at each site: the classifier, the dedupe and the
// source-unavailable report all reach for it, and a typo in any of them would
// produce a lane nothing joins up — silently, because each half would still
// compile.
const sourceWaiting = "customer_waiting"

// sourceTask names the open-task producer. Named for the reason sourceWaiting
// is: the owner filter asks whether a row came from the lane that narrowed to
// one contact in its own query, and a typo there would silently drop every task
// out of the queue it was asked for.
const sourceTask = "task"

// sourceClaim names the rep's own promises. keepUnowned reads it: a claim has
// no assignee column, so the row arrives ownerless while already belonging to
// the rep whose query produced it.
const sourceClaim = "conversation_claim"

// sourceAtRisk names the quiet-deal producer. Three readers spell it — the
// bounds table, the category map and the classifier — which is two more than a
// literal survives.
const sourceAtRisk = "deal_at_risk"

// sourceBriefItem names the overnight queue's own producer.
//
// The brief ranks against the reader's OWN responsibility — their deal, or one
// they hold an open assigned task on — so a row from this lane is already
// bound to the contact asking, exactly as a task or a meeting row is. Judging it
// afterwards by DEAL OWNER would drop the assist case the ranking admitted: the
// night picks a colleague's deal because this rep has work on it, and the owner
// filter then removes it before they ever see it. That is the same starvation
// the ranking fix closed, one layer down.
const sourceBriefItem = "brief_item"

// subjectCompany is the subject type a company-shaped row names.
const subjectCompany = "company"

// subjectDeal is the subject type a deal-shaped row names.
const subjectDeal = "deal"

// subjectContact is the subject type a contact-shaped row names.
//
// A constant for the reason sourceDecay is one: the suppressor pairing the
// decay lane against a waiting row matches on it, and a misspelt literal there
// fails silently — it matches nothing, drops nothing, and reads green.
const subjectContact = "contact"

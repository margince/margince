// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package providerwait is how a deferred row says it waits for the AI provider
// rather than for the budget.
//
// A website read and a voice build stamp status_code 'budget_deferred' for any
// deferral — their CHECK constraints admit no other code — so the free-text
// status_detail is the only column that tells the two waits apart. The writers
// store Detail and the predicates match it, both reading this constant.
package providerwait

// Detail is the status_detail of a row waiting for the provider's next probe.
// It must hold no single quote: the predicates below splice it as a literal.
const Detail = "The AI provider is not answering; this work resumes by itself when it is back."

// Clause selects rows deferred for a provider outage.
const Clause = "status_detail = '" + Detail + "'"

// NotClause selects every other deferral, including a row with no detail.
const NotClause = "status_detail IS DISTINCT FROM '" + Detail + "'"

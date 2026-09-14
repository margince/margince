// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

// The one database refusal whose honest advice is "send it again".
//
// Separate from constraintfault.go because it answers the opposite question.
// That file's whole rule is that a constraint breach is deterministic and must
// never read as retryable; this one is a moment in a deployment, and reading it
// as settled is the same defect pointing the other way.

import (
	"net/http"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// schemaChangedCode is what a client branches on, and it names the caller's
// half of the event rather than ours. "Stale statement cache" is true and is
// about a pool this caller cannot see; what they can act on is that the
// installation's schema moved under a request that was otherwise fine.
const schemaChangedCode = "schema_changed"

// stalePlanFault answers a prepared statement that a migration invalidated.
//
// Without it the refusal is outside the taxonomy, so every surface reports the
// opaque 500 whose advice is to retry — which is, for once, the right advice
// arrived at by the wrong route. The cost is not the status: it is that the
// event is logged as an unknown server fault, an operator goes looking for a
// defect during exactly the window when a deploy explains it, and the agent
// surface tells the caller the call was "settled" and must not be repeated.
// Transient() is the half that matters, and a code is what carries it.
//
// InfraCause and no schema in the sentence, for constraintFault's reason: the
// statement and the relation it was planned against are operator reading.
func stalePlanFault(err error) (Fault, bool) {
	if !storekit.IsStaleStatementCache(err) {
		return Fault{}, false
	}
	return Fault{
		Status: http.StatusServiceUnavailable, Code: schemaChangedCode,
		Detail: "this installation's schema changed while the connection serving this request was " +
			"holding a plan made against the old one. Nothing in the request is wrong and nothing " +
			"was written; send it again — the plan is rebuilt on the next attempt.",
		InfraCause: err,
	}, true
}

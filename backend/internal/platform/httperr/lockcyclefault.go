// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// writeContentionCode names the caller's half of a lock cycle: their write met
// another one at the same moment, and nothing about the request is wrong.
const writeContentionCode = "write_contention"

// lockCycleRetryAfter is the Retry-After, in seconds, a contention refusal
// carries. The other side of a cycle is a short write that is long finished a
// second later; the store has already retried inside that window.
const lockCycleRetryAfter = 1

// lockCycleFault answers a deadlock (40P01) or serialization failure (40001)
// that outlasted the store's own retries. Unmapped, it is an opaque 500 logged
// as an unknown fault and told to an agent as settled — the one advice that is
// wrong, since the same call a moment later is exactly what succeeds.
func lockCycleFault(err error) (Fault, bool) {
	if !storekit.IsLockCycle(err) {
		return Fault{}, false
	}
	return Fault{
		Status: http.StatusServiceUnavailable, Code: writeContentionCode,
		Detail: "this write met a concurrent write to the same records and was rolled back to let the " +
			"other finish. Nothing in the request is wrong and nothing was written; send it again.",
		RetryAfterSeconds: lockCycleRetryAfter,
		InfraCause:        err,
	}, true
}

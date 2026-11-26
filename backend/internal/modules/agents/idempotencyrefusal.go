// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// claimInFlightError refuses a call whose key an earlier call still holds.
// That call may yet write, so a caller that drops the key risks a second
// write. The dispatcher marks the result with the in-progress code, so a client
// keeps the key without reading the sentence.
type claimInFlightError struct{ tool string }

func (e *claimInFlightError) Error() string {
	return fmt.Sprintf("an earlier %s call with this idempotency_key has not finished yet; wait for it rather than "+
		"repeating it: %s", e.tool, apperrors.ErrConflict)
}

func (*claimInFlightError) Unwrap() error { return apperrors.ErrConflict }

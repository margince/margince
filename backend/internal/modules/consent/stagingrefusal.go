// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// StagingRefusal is the error a send refused at staging carries, nil when no
// recipient is denied. It names the refusal the review and the 409 rank first,
// so a log and a client without the review read the same cause.
func StagingRefusal(set commsauthz.DecisionSet) error {
	denied := set.Denied()
	if len(denied) == 0 {
		return nil
	}
	cause := denied[0].ReasonCode
	if blocking := BlockingRefusals(set); len(blocking) > 0 {
		cause = blocking[0].ReasonCode
	}
	return fmt.Errorf("%d of %d recipients may not be written to (%s): %w",
		len(denied), len(set.Decisions), cause, apperrors.ErrConsentNotGranted)
}

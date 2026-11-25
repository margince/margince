// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unactionableIfGone is how a bus consumer reports that the record its event
// names is gone. The consumers run as the system with every row in scope. So a
// store's ErrNotFound means the row was deleted or archived after the event was
// staged, and no redelivery finds it. Any other error passes through and is
// retried.
func unactionableIfGone(err error, kind string, id ids.UUID) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return fmt.Errorf("%w: %s %s", events.ErrUnactionable, kind, id)
	}
	return err
}

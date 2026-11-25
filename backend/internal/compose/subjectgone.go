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

// unactionableIfGone turns a store's ErrNotFound into events.ErrUnactionable,
// so the bus acks the entry once. Any other error passes through and is
// retried. Call it only where ErrNotFound means no retry can succeed.
func unactionableIfGone(err error, kind string, id ids.UUID) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return fmt.Errorf("%w: %s %s", events.ErrUnactionable, kind, id)
	}
	return err
}

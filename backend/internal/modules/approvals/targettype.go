// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/targettype"
)

// requireKnownTargetType refuses a staging whose target type the schema CHECK
// would refuse, so the caller reads a validation answer and not a constraint
// error. An empty type is a staging with no target on that side.
func requireKnownTargetType(column, targetType string) error {
	if targetType == "" || targettype.Known(targetType) {
		return nil
	}
	return fmt.Errorf("%s %q is not a type an approval can name: %w", column, targetType, apperrors.ErrInvalidArgument)
}

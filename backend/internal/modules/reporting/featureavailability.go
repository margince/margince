// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"fmt"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

var errFeatureUnavailable = fmt.Errorf("this report needs a feature that is off: %w", apperrors.ErrNotFound)

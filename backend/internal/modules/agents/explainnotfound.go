// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"errors"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// notFoundExplanation says what is missing. A not-found that names its own
// reason, such as a brief not made yet, is not about access. Told about row
// scope, a model hedges about a record that does not exist.
func notFoundExplanation(err error) string {
	if fault, ok := errors.AsType[apperrors.MessageFault](err); ok {
		_, message := fault.MessageFault()
		return echoSafe(message, MaxFaultDetail)
	}
	return "No such record in this workspace (or it is outside the acting user's row scope). (" + refusalDetail(err) + ")"
}

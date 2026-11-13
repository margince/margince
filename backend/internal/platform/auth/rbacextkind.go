// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// gateObject maps an extension kind to its declared object; an undeclared one is no record, so even the system principal is refused.
func gateObject(kind string) (string, error) {
	if !recordkinds.IsExtensionName(kind) {
		return kind, nil
	}
	declared, ok := recordkinds.Lookup(kind)
	if !ok {
		return "", fmt.Errorf("%s: no extension declares this record kind: %w", kind, apperrors.ErrPermissionDenied)
	}
	return declared.Object, nil
}

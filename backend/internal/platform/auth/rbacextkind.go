// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// gateObject maps an extension kind to its declared object; an undeclared one is no record, so even the system principal is refused.
func gateObject(kind string) (string, error) {
	if !recordkinds.IsExtensionName(kind) {
		return kind, nil
	}
	declared, ok := recordkinds.Lookup(kind)
	if !ok {
		return "", fmt.Errorf("no extension declares the record kind %q: %w", kind, apperrors.ErrPermissionDenied)
	}
	return declared.Object, nil
}

// ReadableKind is an extension kind the caller can read: a unit still declares
// it and the caller holds its read right. A removed kind and a kind the caller
// may not read answer alike, so no reader can probe which kinds exist.
func ReadableKind(ctx context.Context, kind string) (recordkinds.Declared, bool) {
	declared, ok := recordkinds.Lookup(kind)
	if !ok || !Allows(ctx, kind, principal.ActionRead) {
		return recordkinds.Declared{}, false
	}
	return declared, true
}

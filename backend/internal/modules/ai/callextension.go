// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// extensionOf is what ai_call.extension holds for a call under ctx: the unit
// the invocation is bound to, empty for a call no extension made. Provenance
// binds the unit per invocation, so a handler cannot supply it.
func extensionOf(ctx context.Context) string {
	ext, ok := provenance.ExtensionFrom(ctx)
	if !ok {
		return ""
	}
	return ext.Unit
}

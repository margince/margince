// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

type extensionTaskKey struct{}

// WithExtensionTask names the extension task a call under ctx runs, so the
// router writes it on the ai_call row beside the unit. The unit never comes
// from here. Provenance binds it per invocation, and a handler cannot supply it.
func WithExtensionTask(ctx context.Context, task string) context.Context {
	return context.WithValue(ctx, extensionTaskKey{}, task)
}

// extensionOf is what ai_call.extension holds for a call under ctx: the unit,
// then "/" and the declared task name when one was named. Empty for a call no
// extension made.
func extensionOf(ctx context.Context) string {
	ext, ok := provenance.ExtensionFrom(ctx)
	if !ok || ext.Unit == "" {
		return ""
	}
	if task, _ := ctx.Value(extensionTaskKey{}).(string); task != "" {
		return ext.Unit + "/" + task
	}
	return ext.Unit
}

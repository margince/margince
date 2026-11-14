// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a unit uses from the core: the gate that checks its declaration, and the
// bindings behind it. The translation and the limits are in extcapability.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/pkg/extension"
)

// BindExtensionModels records the ModelPath.ExtensionCall lane a unit's Models
// reaches. A nil lane leaves Models refusing.
func BindExtensionModels(lane completer) {
	extensionRuntimeDeps.mu.Lock()
	defer extensionRuntimeDeps.mu.Unlock()
	extensionRuntimeDeps.models = lane
}

// BindExtensionObjects records the store a unit's Objects reaches.
func BindExtensionObjects(store blobstore.Store) {
	extensionRuntimeDeps.mu.Lock()
	defer extensionRuntimeDeps.mu.Unlock()
	extensionRuntimeDeps.blob = store
}

// BindExtensionObjectsFromConfig opens the configured object store and binds it.
// A role calls it before RecordComposition, which sweeps the objects of a unit
// the installation no longer composes.
func BindExtensionObjectsFromConfig(ctx context.Context, settings *config.Config) error {
	store, err := extcapability.OpenStore(ctx, settings)
	if err != nil {
		return fmt.Errorf("compose: opening the object store for extensions: %w", err)
	}
	BindExtensionObjects(store)
	return nil
}

// needed is the gate every capability passes: the Runtime's lifetime and scope,
// then the unit's declaration. The composition is read on each call, so a unit
// that stops declaring a need loses it on its next call.
func (r *callRuntime) needed(ctx context.Context, capability extension.Capability) (context.Context, error) {
	ctx, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	for _, ext := range ComposedExtensions() {
		if string(ext.Name) == r.unit && ext.Declares(capability) {
			return ctx, nil
		}
	}
	return nil, fmt.Errorf("%w: add extension.Need %q to it", extension.ErrNotDeclared, capability)
}

//nolint:ireturn // returning the published port IS the seam: a unit holds extension.Models, never a core type.
func (r *callRuntime) Models() extension.Models {
	return extcapability.Models{
		Lane: r.deps.models, Unit: r.unit,
		Gate:  func(ctx context.Context) (context.Context, error) { return r.needed(ctx, extension.CapabilityModels) },
		Trace: func(ctx context.Context) error { return extcapability.Trace(ctx, r.deps.pool, r.unit) },
	}
}

//nolint:ireturn // returning the published port IS the seam: a unit holds extension.Objects, never a core type.
func (r *callRuntime) Objects() extension.Objects {
	return extcapability.Objects{
		Store: r.deps.blob, Unit: r.unit,
		Gate: func(ctx context.Context) (context.Context, error) { return r.needed(ctx, extension.CapabilityObjects) },
	}
}

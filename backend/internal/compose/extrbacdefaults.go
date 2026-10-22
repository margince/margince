// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The starting grants a unit declares for its RBAC objects.
//
// An operation declares `x-rbac-defaults` beside the `x-rbac-object` it gates
// on. The generator re-emits it as a literal on extension.Verb. This file turns
// the composed verbs into one list. The boot registers it with identity, which
// owns the role set, and applies it once the installation is bound.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/pkg/extension"
)

// extensionRbacDefaults collects the declared default grants, one entry per
// object and role, in the verbs' own order. It refuses a role that is not a
// system role, an action outside the four verbs, and operations that state
// different defaults for one object.
//
// The generator already refuses these, so this is the second line of defence for
// a composition that did not come from it. Operations of one unit share an
// object, and any of them may carry the block, so each object is read once.
func extensionRbacDefaults(verbs []extension.Verb) ([]identity.RbacDefault, error) {
	if err := extension.CheckRbacDefaultsAgree(verbs); err != nil {
		return nil, fmt.Errorf("compose: %w", err)
	}
	declared := map[string]bool{}
	var out []identity.RbacDefault
	for _, v := range verbs {
		if len(v.RbacDefaults) == 0 || declared[v.RbacObject] {
			continue
		}
		declared[v.RbacObject] = true
		for _, d := range v.RbacDefaults {
			actions := make([]string, 0, len(d.Actions))
			for _, a := range d.Actions {
				actions = append(actions, string(a))
			}
			out = append(out, identity.RbacDefault{Object: identity.RbacObject(v.RbacObject), Role: d.Role, Actions: actions})
		}
	}
	if err := identity.ValidateRbacDefaults(out...); err != nil {
		return nil, fmt.Errorf("compose: %w", err)
	}
	return out, nil
}

// registerRbacDefaults hands the declared defaults to identity, after the
// objects they grant on are registered. Validate-then-apply like the objects: a
// set refused in the validate phase never reaches here.
func registerRbacDefaults(defaults []identity.RbacDefault) error {
	if len(defaults) == 0 {
		return nil
	}
	if err := identity.RegisterRbacDefaults(defaults...); err != nil {
		return fmt.Errorf("compose: %w", err)
	}
	return nil
}

// FillExtensionRbacDefaults writes the registered default grants into the
// system roles that have no entry for the object. Every process role calls it
// once the installation is bound. The fill is idempotent and serialized, so the
// api and the worker booting together write each entry once.
//
// Before bootstrap there is no role set to fill. That is "not yet", not an
// error, because the seed writes the registered defaults with the role set.
func FillExtensionRbacDefaults(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:rbac-defaults")
	if err != nil || !bootstrapped {
		return err
	}
	filled, err := identity.NewService(pool).FillRbacDefaults(ctx)
	if err != nil {
		return err
	}
	// Logged after the commit, so it never reports writes that rolled back.
	if filled > 0 {
		log.Info("extension default grants filled", "entries", filled)
	}
	return nil
}

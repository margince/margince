// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The core's half of extension.Core.Members: who holds a role, for a unit that
// declared the need. The admin who installs the unit sees the need and its
// reason, so the roster never opens to a unit nobody agreed it to.

import (
	"context"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/pkg/extension"
)

type extensionMembers struct{ core extensionCore }

//nolint:ireturn // returning the published repo IS the seam: a unit holds extension.MemberRepo, never a core type.
func (c extensionCore) Members() extension.MemberRepo { return extensionMembers{core: c} }

// List answers the active members the query names. A unit that did not declare
// CapabilityMembers is refused before anything is read. A tick, or a delivery
// with no service member, is refused as for any core read.
func (m extensionMembers) List(ctx context.Context, q extension.MemberQuery) ([]extension.Member, error) {
	if !unitDeclares(m.core.unit, extension.CapabilityMembers) {
		return nil, fmt.Errorf("%w: add extension.Need %q to it", extension.ErrNotDeclared, extension.CapabilityMembers)
	}
	ctx, err := m.core.beginRead(ctx)
	if err != nil {
		return nil, err
	}
	found, err := identity.RoleMembersIn(ctx, m.core.tx, q.Role, extension.MaxRosterMembers)
	if err != nil {
		return nil, portRefusal(err)
	}
	out := make([]extension.Member, 0, len(found))
	for _, f := range found {
		out = append(out, extension.Member{ID: f.ID.String(), DisplayName: f.DisplayName, IsAgent: f.IsAgent})
	}
	return out, nil
}

// unitDeclares reads the composition on each call, as callRuntime.needed does,
// so a unit that stops declaring a need loses it on its next call.
func unitDeclares(unit string, capability extension.Capability) bool {
	return slices.ContainsFunc(ComposedExtensions(), func(e extension.Extension) bool {
		return string(e.Name) == unit && e.Declares(capability)
	})
}

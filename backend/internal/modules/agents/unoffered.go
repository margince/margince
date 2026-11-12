// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"sync/atomic"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// servesUnoffered lets tools/call serve a name tools/list leaves out. Only the
// integration build sets it, for the role sweep and the suites that call each
// verb by name. A shipped binary always refuses such a name at the door.
var servesUnoffered atomic.Bool

// unoffered reports a name an agent calls that is not on the public MCP
// surface. It answers as a name that does not exist, so the refusal reveals
// nothing hidden. Whether the caller may use a surface tool is its gate's
// answer, not the door's. The door binds only agents; another caller meets each
// tool's own gate.
func (s *Dispatcher) unoffered(ctx context.Context, name string) bool {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalAgent || servesUnoffered.Load() {
		return false
	}
	return !s.registry.OnSurface(ctx, name)
}

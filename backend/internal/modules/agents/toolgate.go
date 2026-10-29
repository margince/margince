// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ToolGate answers whether a Gated tool exists for the caller in ctx. A tool it
// refuses is neither listed nor callable, and a call by name gets the same
// answer as a name that was never registered.
type ToolGate func(ctx context.Context, tool string) bool

// WithToolGate injects the predicate that decides who a Gated tool exists for.
// The tool list and the call path read the same predicate.
func WithToolGate(gate ToolGate) RegistryOption {
	return func(r *Registry) { r.toolGate = gate }
}

func (r *Registry) offersGated(ctx context.Context, spec mcp.ToolSpec) bool {
	return r.toolGate != nil && r.toolGate(ctx, spec.Name)
}

// gatedFor lists the Gated tools the gate offers this caller. Runnable skips
// them, so a scheduled run never holds one.
func (r *Registry) gatedFor(ctx context.Context, all []mcp.ToolSpec) []mcp.ToolSpec {
	var out []mcp.ToolSpec
	for _, spec := range all {
		if spec.Gated && invocableByCaller(ctx, spec) && r.offersGated(ctx, spec) {
			out = append(out, r.asSeenBy(ctx, spec))
		}
	}
	return out
}

// assertProtocolOnlyShape holds ProtocolOnly to what it is for. The flag skips
// both REST parity gates. It is refused on a listed tool, and on a tool that
// names a REST route it should be held to.
func assertProtocolOnlyShape(spec mcp.ToolSpec) {
	if !spec.ProtocolOnly {
		return
	}
	if !spec.Gated || !spec.Unlisted || spec.OpenAPIOp != "" {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: %s is ProtocolOnly, which needs Gated, Unlisted and no OpenAPIOp: "+
			"a verb with a REST route is held to it, and a listed verb is not a protocol probe", spec.Name))
	}
}

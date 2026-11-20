// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"fmt"
	"maps"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// WithRouteRetryKeys names the route-backed verbs whose REST route claims a
// caller's Idempotency-Key and replays its recorded answer, re-checked against
// the caller as they are now. Such a verb takes `idempotency_key` and hands it
// to its route, so one claim, keyed on one path, serves both doors.
func WithRouteRetryKeys(verbs map[string]bool) RegistryOption {
	return func(r *Registry) { r.routeKeyed = maps.Clone(verbs) }
}

// routeRetryKey carries a caller's key to the route that claims it.
type routeRetryKey struct{}

// RouteRetryKey is the caller's key for a route-backed call whose route claims
// it, and empty for every other call.
func RouteRetryKey(ctx context.Context) string {
	key, _ := ctx.Value(routeRetryKey{}).(string)
	return key
}

// handKeyToRoute moves the key of a call whose route claims it from this
// surface's own claim onto the context the route runs under. The surface then
// claims nothing, so the route's claim is the only one and a retry through
// either door replays the same answer.
//
// The route claims only once the call runs, after any approval is redeemed. So a
// keyed call with an approval is refused. Its retry would meet the spent approval
// before the route could replay the first answer.
func (r *Registry) handKeyToRoute(ctx context.Context, spec mcp.ToolSpec, res reserved) (context.Context, reserved, error) {
	if res.RetryKey == "" {
		return ctx, res, nil
	}
	r.mu.RLock()
	_, runsRoute := r.tools[spec.Name].(routeVerbTool)
	keyed := runsRoute && r.routeKeyed[spec.Name]
	r.mu.RUnlock()
	if !keyed {
		return ctx, res, nil
	}
	if !res.ApprovalID.IsZero() {
		return ctx, res, &BadArgsError{Cause: fmt.Errorf(
			"%s runs at once and is never staged, so it takes `%s` or `approval_id`, not both; repeat it with `%s` alone",
			spec.Name, idempotencyKeyArg, idempotencyKeyArg)}
	}
	ctx = context.WithValue(ctx, routeRetryKey{}, res.RetryKey)
	res.RetryKey = ""
	return ctx, res, nil
}

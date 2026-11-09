// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"net/url"
)

type redirectGuardKey struct{}

// WithRedirectGuard narrows where the fetches made with ctx may be redirected: a hop the
// guard refuses fails the fetch. A caller that checked the first URL holds every hop to it.
func WithRedirectGuard(ctx context.Context, guard func(*url.URL) error) context.Context {
	return context.WithValue(ctx, redirectGuardKey{}, guard)
}

// RedirectAllowed answers whether ctx's guard admits a hop to target; nil without a guard.
func RedirectAllowed(ctx context.Context, target *url.URL) error {
	guard, ok := ctx.Value(redirectGuardKey{}).(func(*url.URL) error)
	if !ok {
		return nil
	}
	return guard(target)
}

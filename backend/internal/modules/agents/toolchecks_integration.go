// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agents

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// WithChecksRecordedForTest hands sink, after each call, the verb that ran and
// every check a gate asked of the caller on its way. It lives behind the
// integration tag, so no shipped binary links it.
func WithChecksRecordedForTest(sink func(verb string, checks []string)) RegistryOption {
	return func(r *Registry) {
		r.recordCall = func(ctx context.Context, verb string) (context.Context, func()) {
			recorded, checks := auth.RecordChecksForTest(ctx)
			return recorded, func() { sink(verb, checks()) }
		}
	}
}

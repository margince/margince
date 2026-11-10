// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package corefeatures

import "context"

type availabilityKey struct{}

// Context bounds public requests and tool calls without changing stored role grants.
// Maintenance without this context can still work on retained records.
func (a Availability) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, availabilityKey{}, a)
}

func FromContext(ctx context.Context) Availability {
	availability, ok := ctx.Value(availabilityKey{}).(Availability)
	if !ok {
		return Availability{}
	}
	return availability
}

// Privacy duties still require their normal grants when a feature is hidden.
func RetainedDataContext(ctx context.Context) context.Context {
	return (Availability{}).Context(ctx)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orderedfetch

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// Reservation is memory one fetch holds from a process budget. A nil one
// holds nothing, and all its methods are safe to call.
type Reservation struct {
	budget *semaphore.Weighted
	held   int64
}

type reservationKey struct{}

// Reserve waits for bytes from budget. A nil budget reserves nothing.
func Reserve(ctx context.Context, budget *semaphore.Weighted, bytes int64) (*Reservation, error) {
	if budget == nil {
		return nil, nil //nolint:nilnil // a walk with no budget retains nothing to account
	}
	if err := budget.Acquire(ctx, bytes); err != nil {
		return nil, err
	}
	return &Reservation{budget: budget, held: bytes}, nil
}

// Context carries the reservation to the fetch, so a reader that learns the
// real size can shrink it early.
func (r *Reservation) Context(ctx context.Context) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, reservationKey{}, r)
}

// FromContext answers the reservation Context put on ctx.
func FromContext(ctx context.Context) (*Reservation, bool) {
	r, ok := ctx.Value(reservationKey{}).(*Reservation)
	return r, ok
}

// Shrink gives back everything above bytes.
func (r *Reservation) Shrink(bytes int64) {
	if r != nil && bytes < r.held {
		r.budget.Release(r.held - bytes)
		r.held = bytes
	}
}

// Release gives back the whole reservation.
func (r *Reservation) Release() {
	if r != nil {
		r.Shrink(0)
	}
}

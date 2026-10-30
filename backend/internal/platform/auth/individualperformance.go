// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Whether a reader may measure a colleague one by one.
//
// Row scope and the analytics lens answer whose work a seat may measure. This
// answers whether the installation lets anyone measure a colleague at all,
// which a works council may rule out for every role at once. A reader may
// always measure themselves.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type individualPerformanceKey struct{}

// WithIndividualPerformance binds the installation's switch for one request or
// one job run, so every check inside it reads one answer.
func WithIndividualPerformance(ctx context.Context, allowed bool) context.Context {
	return context.WithValue(ctx, individualPerformanceKey{}, allowed)
}

// IndividualPerformanceOf answers the bound switch. ok is false when nothing
// bound it; the caller then reads the setting itself.
func IndividualPerformanceOf(ctx context.Context) (allowed, ok bool) {
	allowed, ok = ctx.Value(individualPerformanceKey{}).(bool)
	return allowed, ok
}

// MayMeasureSeat is the rule: the reader themselves always, anyone else only
// while the installation allows it.
func MayMeasureSeat(ctx context.Context, allowed bool, seat ids.UUID) bool {
	if allowed {
		return true
	}
	p, ok := principal.Actor(ctx)
	return ok && !seat.IsZero() && seat == p.UserID
}

// IndividualPerformanceOffError refuses a question about a colleague, or one that splits
// its answer by seat, while the installation measures nobody one by one.
type IndividualPerformanceOffError struct{}

func (IndividualPerformanceOffError) Error() string {
	return "auth: individual performance is switched off for this installation"
}

// Is keeps the refusal on the permission sentinel's 403 row.
func (IndividualPerformanceOffError) Is(target error) bool {
	return target == apperrors.ErrPermissionDenied
}

// MessageFault says what the reader can still ask.
func (IndividualPerformanceOffError) MessageFault() (code, message string) {
	return "individual_performance_off", "An administrator switched off individual performance. " +
		"You can see your own figures, and totals that at least two other members contribute to."
}

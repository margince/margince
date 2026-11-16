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
	own, err := OwnSeat(ctx)
	return err == nil && seat == own
}

// OwnSeat is the reader's own seat, the one the switch never withholds. A
// caller cannot name another, so a record read by it is the reader's own.
func OwnSeat(ctx context.Context) (ids.UUID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID.IsZero() {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return p.UserID, nil
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
		"You can see your own figures. " +
		"A figure is withheld when fewer than two other members make it up, or when taking another team's total from it would leave fewer than two. " +
		"A forecast is withheld in full if any of its groups would show one colleague's figures."
}

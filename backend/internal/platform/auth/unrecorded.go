// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/corefeatures"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Unrecorded asks Require's question and answers its error, for a read whose
// refusal the call degrades over, such as a section it withholds. The ctx it
// returns carries no recorder, so the gates of that read record nothing: the
// call does not need what they ask.
func Unrecorded(ctx context.Context, object string, action principal.Action) (context.Context, error) {
	if err := probe(ctx, object, action); err != nil {
		return ctx, err
	}
	return withoutRecorder(ctx), nil
}

// withoutRecorder is ctx with no recorder, for a gate asked as a probe whose
// refusal the caller turns into an answer rather than returns.
func withoutRecorder(ctx context.Context) context.Context {
	return context.WithValue(ctx, checkRecorderKey{}, (*checkRecorder)(nil))
}

// probe is Require recording nothing. A branch uses it, because its refusal is
// not the call's answer, and a recorded check hides a tool from its lacker.
func probe(ctx context.Context, object string, action principal.Action) error {
	object, err := gateObject(object)
	if err != nil {
		return err
	}
	return probeCheck(ctx, Check{Object: object, Actions: []principal.Action{action}})
}

func probeCheck(ctx context.Context, c Check) error {
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if p.Type == principal.PrincipalBuyer {
		return refuseBuyer(p, c.String())
	}
	if !corefeatures.FromContext(ctx).RecordEnabled(c.Object) || !c.HeldBy(p) {
		return fmt.Errorf("%s: %w", c, apperrors.ErrPermissionDenied)
	}
	return nil
}

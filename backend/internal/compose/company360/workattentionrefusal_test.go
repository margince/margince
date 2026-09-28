// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The task read refuses a caller with no seat, rather than answering a
// complete-looking empty set.
//
// Deliberately NOT a permission denial. readWorkAttention folds
// ErrPermissionDenied into attention_withheld — rows with no reasons, and a
// payload saying so — which is right for a reader whose grants stop at the
// activity lane. A caller with no actor at all is a programming error, and
// folding it would draw a page for a request that never had a seat.
//
// The read now reports what it could not show as well as what it could, and
// both halves of that answer come from the scope clause. A caller the clause
// cannot be built for has no answer at all — neither the rows nor the claim
// that there were none — so the refusal has to reach the caller instead of
// being folded into "nothing outstanding", which is the exact reading this
// section exists to prevent.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheOverdueTaskReadRefusesACallerWithNoSeat(t *testing.T) {
	t.Parallel()
	// No principal on the context, so the activity scope cannot be composed.
	// The transaction is never reached: the clause is built before any query,
	// which is what makes this answerable without a database.
	tasks, complete, err := overdueTasksBy(
		context.Background(), nil, ids.New[ids.CompanyKind](),
		"deal_id", []ids.UUID{ids.NewV7()}, time.Now())

	if err == nil {
		t.Fatal("a caller with no seat got an answer — an empty task set reads as a piece of work " +
			"with nothing outstanding, which is what this section must never say by accident")
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("err = %v — folded into attention_withheld, so a request with no seat "+
			"would draw a page instead of being refused", err)
	}
	if tasks != nil {
		t.Errorf("a refusal carried %d task(s)", len(tasks))
	}
	if complete {
		t.Error("a refusal claimed the answer was complete")
	}
}

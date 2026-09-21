// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam's fail-closed arm.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An entity type no owner claims is refused rather than silently doing
// nothing.
//
// The generator types entityType as a closed enum, so this cannot arrive over
// HTTP today — which is exactly why it is worth a test rather than a comment
// saying so. If the enum gains a sixth record and this switch does not, the
// contract will accept the call and the seam must not answer 204 to a hold it
// never placed. Nothing else in the product would notice.
func TestTheHoldSeamRefusesARecordTypeNoModuleOwns(t *testing.T) {
	t.Parallel()
	// A zero-valued seam: the refusal has to land before any store is reached,
	// so a version that dispatched first would panic here rather than pass.
	err := LegalHoldSeam{}.SetLegalHold(
		context.Background(), "invoice", ids.NewV7(), true, "Anwaltsschreiben 2026-14")
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("holding an unowned record type returned %v, want ErrInvalidArgument", err)
	}
}

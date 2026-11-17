// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAFileNamedTwiceIsRefusedBeforeAnythingIsClaimed(t *testing.T) {
	file := ids.NewV7()
	_, _, err := claimUploadsTx(context.Background(), nil, ids.NewV7(), []ids.UUID{file, file})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("claiming one upload twice = %v, want an invalid-argument refusal", err)
	}
}

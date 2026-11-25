// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAMissingRecordIsUnactionableAndEveryOtherErrorIsRetried(t *testing.T) {
	deal := ids.NewV7()
	gone := UnactionableIfGone(fmt.Errorf("writing the note: %w", apperrors.ErrNotFound), "deal", deal)
	if !errors.Is(gone, ErrUnactionable) {
		t.Errorf("a wrapped ErrNotFound became %v, want ErrUnactionable so the entry is acked", gone)
	}

	transient := errors.New("connection reset")
	if got := UnactionableIfGone(transient, "deal", deal); !errors.Is(got, transient) || errors.Is(got, ErrUnactionable) {
		t.Errorf("a transient error became %v, want it unchanged so the entry is retried", got)
	}
	if got := UnactionableIfGone(apperrors.ErrPermissionDenied, "deal", deal); errors.Is(got, ErrUnactionable) {
		t.Errorf("a refusal became %v, want it unchanged", got)
	}
	if got := UnactionableIfGone(nil, "deal", deal); got != nil {
		t.Errorf("success became %v, want nil", got)
	}
}

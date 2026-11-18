// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A repeat press is marked without changing what the refusal says or is: the
// 409 mapping and every errors.Is on the refusal read through the mark.
func TestARepeatedSendKeepsTheRefusalItWraps(t *testing.T) {
	refusal := fmt.Errorf("consent: 1 of 1 recipients may not be written to: %w", apperrors.ErrConsentNotGranted)
	repeated := error(&repeatedSendError{refusal: refusal})

	if !IsRepeatedSend(repeated) || !IsRepeatedSend(fmt.Errorf("send: %w", repeated)) {
		t.Fatal("a refusal of an already held message does not read as a repeat")
	}
	if IsRepeatedSend(refusal) {
		t.Fatal("a first refusal reads as a repeat")
	}
	if !errors.Is(repeated, apperrors.ErrConsentNotGranted) || repeated.Error() != refusal.Error() {
		t.Fatalf("the mark changed the refusal: %v", repeated)
	}
}

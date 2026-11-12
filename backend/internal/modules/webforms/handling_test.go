// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestABusyFormIsRetryableWithoutChangingTheSubmission(t *testing.T) {
	fault, ok := httperr.Classify(refusal(ErrHandlingBusy))
	if !ok || fault.Status != http.StatusServiceUnavailable || !fault.Transient() {
		t.Fatalf("busy form refusal = %+v (classified %v)", fault, ok)
	}
}

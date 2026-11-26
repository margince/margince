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
	refusal := fmt.Errorf("1 of 1 recipients may not be written to: %w", apperrors.ErrConsentNotGranted)
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

// referencedRefusal stands in for the consent module's refusal, which this
// module may not import: a refusal that carries the review it opened.
type referencedRefusal struct{ cause error }

func (r *referencedRefusal) Error() string                  { return r.cause.Error() }
func (r *referencedRefusal) Unwrap() error                  { return r.cause }
func (r *referencedRefusal) Unreferenced() error            { return r.cause }
func (r *referencedRefusal) FaultReference() map[string]any { return map[string]any{"review_id": "r"} }

// The 409 renderer finds the refusal's review through the mark, by its type
// and by the referenced-fault interface it asks for.
func TestARepeatedSendStillCarriesTheReviewItsRefusalOpened(t *testing.T) {
	refusal := &referencedRefusal{cause: apperrors.ErrConsentNotGranted}
	repeated := error(&repeatedSendError{refusal: refusal})

	if found, ok := errors.AsType[*referencedRefusal](repeated); !ok || found != refusal {
		t.Fatal("the refusal's own type is lost behind the repeat mark")
	}
	if found, ok := errors.AsType[apperrors.ReferencedFault](repeated); !ok || found.FaultReference()["review_id"] != "r" {
		t.Fatal("the review reference is lost behind the repeat mark")
	}
}

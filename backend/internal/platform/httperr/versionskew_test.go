// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A lost race reads the same to every caller, however a store wrapped it. The
// wrap names a code path; the reader needs what happened and what to do.
func TestAWrappedVersionSkewAnswersTheReadersSentence(t *testing.T) {
	for _, err := range []error{
		apperrors.ErrVersionSkew,
		fmt.Errorf("apply product patch: %w", apperrors.ErrVersionSkew),
		fmt.Errorf("apply offer patch: %w", fmt.Errorf("guarded: %w", apperrors.ErrVersionSkew)),
	} {
		fault, ok := Classify(err)
		if !ok {
			t.Fatalf("%v is not classified", err)
		}
		if fault.Code != "version_skew" || fault.Detail != versionSkewDetail {
			t.Errorf("%v answered %s %q, want version_skew with the reader's sentence", err, fault.Code, fault.Detail)
		}
	}
}

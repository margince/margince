// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A plain wrap of a lost race names a code path, so the reader gets the
// generic sentence of what happened and what to do.
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

// A message written for the reader says what the generic sentence cannot,
// such as re-running a refresh rather than reloading, so it reaches them unchanged.
func TestAVersionSkewWrittenForTheReaderKeepsItsWords(t *testing.T) {
	advice := &apperrors.VersionSkewError{Message: "the EUR rate changed since the proposal was diffed — re-run the refresh"}
	for _, err := range []error{advice, fmt.Errorf("release approval: %w", advice)} {
		fault, ok := Classify(err)
		if !ok {
			t.Fatalf("%v is not classified", err)
		}
		if fault.Status != http.StatusConflict || fault.Code != "version_skew" || fault.Detail != advice.Message {
			t.Errorf("%v answered %d %s %q, want 409 version_skew with %q",
				err, fault.Status, fault.Code, fault.Detail, advice.Message)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestABriefNotMadeYetIsANotFoundThatSaysWhy(t *testing.T) {
	err := error(NoBriefYetError{})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("%v does not unwrap to not-found, so a caller that branches on it breaks", err)
	}
	fault, ok := httperr.Classify(err)
	if !ok {
		t.Fatalf("%v does not classify", err)
	}
	if fault.Status != http.StatusNotFound || fault.Code != "not_found" || !strings.Contains(fault.Detail, "made for today yet") {
		t.Errorf("answered %d %s %q, want a 404 not_found that says no brief is made yet", fault.Status, fault.Code, fault.Detail)
	}
}

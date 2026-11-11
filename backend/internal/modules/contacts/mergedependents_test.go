// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAMergeRefusedForDependentsIsAConflictThatNamesTheCountAndArchive(t *testing.T) {
	var err error = &DependentRecordsError{Kind: "contact", Count: 3}

	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("the refusal is not a conflict: %v", err)
	}
	if !strings.Contains(err.Error(), "3 extension records") {
		t.Errorf("Error() = %q; want the count", err.Error())
	}
	var fault interface{ MessageFault() (string, string) }
	if !errors.As(err, &fault) {
		t.Fatal("the refusal carries no message for the caller")
	}
	code, message := fault.MessageFault()
	if code != "has_dependent_records" || !strings.Contains(message, "3 extension records") || !strings.Contains(message, "archive it instead") {
		t.Errorf("MessageFault() = %q, %q; want the code, the count and the way forward", code, message)
	}
}

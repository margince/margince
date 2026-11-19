// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The refusal is a conflict that says how to go on. It names no invoice, so a
// reader without finance access learns nothing about the ledger.
func TestAMergeRefusedForUnlinkedLedgerRowsSaysHowToGoOnAndNamesNoInvoice(t *testing.T) {
	var err error = &UnlinkedLedgerRowsError{}
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("the refusal is not a conflict: %v", err)
	}
	var fault apperrors.MessageFault
	if !errors.As(err, &fault) {
		t.Fatal("the refusal carries no message for the caller")
	}
	code, message := fault.MessageFault()
	if code != "company_merge_records_cannot_move" {
		t.Errorf("code = %q, want company_merge_records_cannot_move", code)
	}
	for _, remedy := range []string{"Keep this company as the one that stays", "link its accounting customer first"} {
		if !strings.Contains(message, remedy) {
			t.Errorf("message %q does not say %q", message, remedy)
		}
	}
	for _, disclosure := range []string{"invoice", "payment"} {
		if strings.Contains(strings.ToLower(message), disclosure) {
			t.Errorf("message %q names %q to a reader who may not read finance", message, disclosure)
		}
	}
}

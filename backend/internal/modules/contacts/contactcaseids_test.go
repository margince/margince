// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A case decision that leaves out its survivor or owner is refused naming the
// field, before any lookup can read the zero id as a contact nobody named.
func TestEveryRequiredCaseBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()
	a, b := ids.NewV7(), ids.NewV7()
	whole := ContactCaseDecision{MemberIDs: []ids.UUID{a, b}, SurvivorID: a, OwnerID: ids.NewV7()}
	if _, err := caseSelection(whole); err != nil {
		t.Fatalf("a whole decision was refused: %v", err)
	}
	for field, decision := range map[string]ContactCaseDecision{
		"survivor_id": {MemberIDs: whole.MemberIDs, OwnerID: whole.OwnerID},
		"owner_id":    {MemberIDs: whole.MemberIDs, SurvivorID: a},
	} {
		_, err := caseSelection(decision)
		var refusal *httperr.DetailedError
		if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Field != field {
			t.Errorf("leaving out %s answered %v, want a refusal naming it", field, err)
		}
	}
}

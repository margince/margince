// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAnUndoRefusalThatLeavesTheRowInPlaceIsKeptWithItsReason(t *testing.T) {
	edited := map[string]error{
		"contact":  &contacts.HumanTouchedError{EntityType: "company"},
		"deal":     &deals.HumanTouchedError{},
		"activity": &activities.HumanTouchedError{},
	}
	for name, cause := range edited {
		kept, ok := errors.AsType[*migration.KeptError](keptCause(fmt.Errorf("archiving: %w", cause)))
		if !ok || kept.Why.Code != migration.ReasonUndoEdited {
			t.Errorf("%s: got %+v, want a kept row edited since the import", name, kept)
		}
	}
	kept, ok := errors.AsType[*migration.KeptError](keptCause(&contacts.StillReferencedError{Deals: 2, Contacts: 1}))
	if !ok || kept.Why.Code != migration.ReasonUndoStillReferenced ||
		kept.Why.Params["deals"] != "2" || kept.Why.Params["contacts"] != "1" {
		t.Fatalf("got %+v, want the company kept with the counts that name it", kept)
	}
	plain := fmt.Errorf("rule: %w", apperrors.ErrConflict)
	if _, ok := errors.AsType[*migration.KeptError](keptCause(plain)); ok {
		t.Error("an unclassified conflict must stay an errored row")
	}
}

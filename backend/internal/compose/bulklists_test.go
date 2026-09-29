// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAListVerbNamesItsShortlistAndNoOwner(t *testing.T) {
	list, owner := ids.NewV7(), ids.NewV7()
	one := []crmcontracts.BulkItem{{Id: openapi_types.UUID(ids.NewV7()), Version: 1}}
	for name, tc := range map[string]struct {
		change bulkChange
		field  string
	}{
		"no Shortlist named":   {bulkChange{verb: crmcontracts.BulkVerbAddToList, items: one}, "list_id"},
		"an owner handed in":   {bulkChange{verb: crmcontracts.BulkVerbRemoveFromList, items: one, listID: &list, ownerID: &owner}, "owner_id"},
		"a well-formed change": {bulkChange{verb: crmcontracts.BulkVerbAddToList, items: one, listID: &list}, ""},
	} {
		err := validateBulkChange(tc.change)
		var refused *httperr.DetailedError
		switch {
		case tc.field == "" && err != nil:
			t.Errorf("%s: refused with %v", name, err)
		case tc.field != "" && (!errors.As(err, &refused) || refused.Fields[0].Field != tc.field):
			t.Errorf("%s: %v, want a refusal naming %s", name, err, tc.field)
		}
	}
}

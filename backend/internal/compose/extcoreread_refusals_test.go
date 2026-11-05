// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/pkg/extension"
)

// Every refusal below happens before the database is reached, so the core
// carries no pool and no transaction: a verb that touched either would panic.
func TestAMalformedReadIsRefusedAsInvalidBeforeTheStoreIsTouched(t *testing.T) {
	ctx := context.Background()
	core := attendedCore("probe-unit")
	bad := extension.ReadPage{Limit: -1}

	_, dealGet := core.Deals().Get(ctx, "nope")
	_, companyGet := core.Companies().Get(ctx, "nope")
	_, signalGet := core.Signals().Get(ctx, "nope")
	_, activityGet := core.Activities().Get(ctx, "nope")
	_, dealList := core.Deals().List(ctx, extension.DealQuery{ReadPage: bad})
	_, companyLimit := core.Companies().List(ctx, extension.CompanyQuery{ReadPage: bad})
	_, companyHalf := core.Companies().List(ctx, extension.CompanyQuery{SourceSystem: "erp"})
	_, companyOwner := core.Companies().List(ctx, extension.CompanyQuery{OwnerID: "x"})
	_, signalLimit := core.Signals().List(ctx, extension.SignalQuery{ReadPage: bad})
	_, signalCompany := core.Signals().List(ctx, extension.SignalQuery{CompanyID: "x"})
	_, activityLimit := core.Activities().List(ctx, extension.ActivityQuery{ReadPage: bad})
	_, activityHalf := core.Activities().List(ctx, extension.ActivityQuery{EntityType: "deal"})
	_, activityID := core.Activities().List(ctx, extension.ActivityQuery{EntityType: "deal", EntityID: "x"})

	for name, err := range map[string]error{
		"deal get": dealGet, "company get": companyGet, "signal get": signalGet, "activity get": activityGet,
		"deal list": dealList, "company limit": companyLimit, "company half pair": companyHalf,
		"company owner": companyOwner, "signal limit": signalLimit, "signal company": signalCompany,
		"activity limit": activityLimit, "activity half pair": activityHalf, "activity entity id": activityID,
	} {
		if !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s answered %v, want ErrInvalid", name, err)
		}
	}
}

func TestAListsOwnRefusalsMapToInvalid(t *testing.T) {
	for name, err := range map[string]error{
		"predicate":       &storekit.PredicateError{Field: "cf_x", Code: "unknown", Message: "m"},
		"sort":            &storekit.SortError{Code: "c", Message: "m"},
		"malformed":       &storekit.MalformedCursorError{},
		"cursor mismatch": &storekit.CursorSortMismatchError{},
	} {
		if got := portRefusal(err); !errors.Is(got, extension.ErrInvalid) {
			t.Errorf("%s mapped to %v, want ErrInvalid", name, got)
		}
	}
}

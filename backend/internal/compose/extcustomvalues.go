// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The custom values a unit offers for a company or a deal. Landing a record and
// a handler's SetCustomFields both decide what to write here, so the two cannot
// disagree about which value a user owns.

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// customValuesToWrite answers the offered custom fields that differ from the
// record and that no user last edited, and names the ones a user owns. A field
// has no pairing like amount and currency, so each is held back on its own.
// held names the fields of a patch a user last edited.
func customValuesToWrite(
	ctx context.Context, current, offered map[string]any, held func(context.Context, map[string]any) ([]string, error),
) (map[string]any, []string, error) {
	if len(offered) == 0 {
		return nil, nil, nil
	}
	// The stored value reads back as decoded JSON. The offer is put through JSON
	// too, or a []string would never equal the []any it was saved as.
	raw, err := json.Marshal(offered)
	if err != nil {
		return nil, nil, fmt.Errorf("compose: encoding the offered custom fields: %w", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, nil, fmt.Errorf("compose: reading the offered custom fields back: %w", err)
	}
	changed := map[string]any{}
	for key, value := range normalized {
		if !reflect.DeepEqual(current[key], value) {
			changed[key] = value
		}
	}
	owned, err := held(ctx, changed)
	if err != nil {
		return nil, nil, err
	}
	for _, key := range owned {
		delete(changed, key)
	}
	slices.Sort(owned)
	return changed, owned, nil
}

// SetCustomFields is the unit's door onto a company's custom values, on the
// transaction it holds. The checks and the choice of what to write are the ones
// a landed company record gets. The edit is UpdateCompany's, which holds the
// caller's grant and row scope.
func (c extensionCompanies) SetCustomFields(ctx context.Context, id string, values map[string]any) ([]string, error) {
	if err := c.core.refuseUnattended(); err != nil {
		return nil, err
	}
	ctx, err := c.core.authorised(ctx)
	if err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return nil, portRefusal(err)
	}
	companyID, err := ids.ParseAs[ids.CompanyKind](id)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, id)
	}
	if err := extension.ValidateCustomFieldKeys(values); err != nil {
		return nil, fmt.Errorf("%w: %s", extension.ErrInvalid, err)
	}
	cols, err := contacts.CompanyColumnsFrom(ctx, c.core.catalog())
	if err != nil {
		return nil, portRefusal(err)
	}
	current, err := c.store().GetCompanyTx(ctx, c.core.tx, companyID, storekit.LiveOnly, cols)
	if err != nil {
		return nil, portRefusal(err)
	}
	overwrite, err := overwritesEditedFieldsIn(ctx, c.core.tx, c.core.unit)
	if err != nil {
		return nil, portRefusal(err)
	}
	held := func(ctx context.Context, patch map[string]any) ([]string, error) {
		if overwrite || len(patch) == 0 {
			return nil, nil
		}
		raw, err := json.Marshal(patch)
		if err != nil {
			return nil, err
		}
		return humanOwnedConflictsIn(ctx, c.core.tx, string(recordTypeCompany), companyID.UUID, raw)
	}
	custom, withheld, err := customValuesToWrite(ctx, current.AdditionalProperties, values, held)
	if err != nil {
		return nil, portRefusal(err)
	}
	if len(custom) == 0 {
		return withheld, nil
	}
	if err := c.store().SetCustomFieldsTx(ctx, c.core.tx, companyID, custom, cols); err != nil {
		return nil, portRefusal(err)
	}
	return withheld, nil
}

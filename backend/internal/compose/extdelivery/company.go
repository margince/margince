// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extdelivery

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
	"github.com/margince/margince/backend/pkg/extension"
)

// CompanyDoor is a unit's door onto a company's custom values, on the
// transaction it holds. Refuse maps a core error to a published class.
type CompanyDoor struct {
	Tx         pgx.Tx
	Catalog    fieldcatalog.Reader
	Store      *contacts.Store
	Refuse     func(error) error
	Overwrites func(context.Context) (bool, error)
	Held       func(ctx context.Context, company ids.UUID, patch json.RawMessage) ([]string, error)
}

// SetCustomFields writes the custom values a landed company record would get,
// and names those a user owns. The edit is UpdateCompany's, which holds the
// caller's grant and row scope.
func (d CompanyDoor) SetCustomFields(ctx context.Context, id string, values map[string]any) ([]string, error) {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return nil, d.Refuse(err)
	}
	companyID, err := ids.ParseAs[ids.CompanyKind](id)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a UUID", extension.ErrInvalid, id)
	}
	if err := extension.ValidateCustomFieldKeys(values); err != nil {
		return nil, fmt.Errorf("%w: %s", extension.ErrInvalid, err)
	}
	cols, err := contacts.CompanyColumnsFrom(ctx, d.Catalog)
	if err != nil {
		return nil, d.Refuse(err)
	}
	if err := d.admit(ctx, companyID, cols, values); err != nil {
		return nil, err
	}
	current, err := d.Store.GetCompanyTx(ctx, d.Tx, companyID, storekit.LiveOnly, cols)
	if err != nil {
		return nil, d.Refuse(err)
	}
	overwrite, err := d.Overwrites(ctx)
	if err != nil {
		return nil, d.Refuse(err)
	}
	held := func(ctx context.Context, patch map[string]any) ([]string, error) {
		if overwrite || len(patch) == 0 {
			return nil, nil
		}
		raw, err := json.Marshal(patch)
		if err != nil {
			return nil, err
		}
		return d.Held(ctx, companyID.UUID, raw)
	}
	custom, withheld, err := ValuesToWrite(ctx, current.AdditionalProperties, values, held)
	if err != nil {
		return nil, d.Refuse(err)
	}
	if len(custom) == 0 {
		return withheld, nil
	}
	if err := d.Store.SetCustomFieldsTx(ctx, d.Tx, companyID, custom, cols); err != nil {
		return nil, d.Refuse(err)
	}
	return withheld, nil
}

// admit refuses a key that is no active field or a value its field cannot hold.
// It then holds the company, so the ownership read after it cannot go stale.
func (d CompanyDoor) admit(ctx context.Context, company ids.CompanyID, cols contacts.CustomColumns, values map[string]any) error {
	for key := range values {
		if !cols.Has(key) {
			return fmt.Errorf("%w: %q is not an active custom field", extension.ErrInvalid, key)
		}
	}
	if err := cols.Check(values); err != nil {
		return fmt.Errorf("%w: %s", extension.ErrInvalid, err)
	}
	if err := d.Store.LockCompanyTx(ctx, d.Tx, company); err != nil {
		return d.Refuse(err)
	}
	return nil
}

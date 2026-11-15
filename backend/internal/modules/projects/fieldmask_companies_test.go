// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestFillingNoCompaniesPreservesTheAnchorMaskAndWriteFlag(t *testing.T) {
	for _, writable := range []bool{false, true} {
		masked := []string{"company_id"}
		p := crmcontracts.Project{MaskedFields: &masked, Writable: &writable}
		got, err := fillCompanies(context.Background(), nil, p,
			func(context.Context, pgx.Tx, ids.ProjectID) ([]CompanyOnProject, error) {
				return nil, nil
			})
		if err != nil {
			t.Fatal(err)
		}
		if got.CompanyId != nil || got.Companies == nil || len(*got.Companies) != 0 {
			t.Errorf("company_id = %v, companies = %v, want null and an empty list", got.CompanyId, got.Companies)
		}
		if !reflect.DeepEqual(got.MaskedFields, p.MaskedFields) || !reflect.DeepEqual(got.Writable, p.Writable) {
			t.Fatal("filling the empty company list changed the mask or writable flag")
		}
	}
}

func TestACompanyReadErrorIsNotAnEmptyProjectCompanyList(t *testing.T) {
	failure := errors.New("company read failed")
	_, err := fillCompanies(context.Background(), nil, crmcontracts.Project{},
		func(context.Context, pgx.Tx, ids.ProjectID) ([]CompanyOnProject, error) {
			return nil, failure
		})
	if !errors.Is(err, failure) {
		t.Fatalf("fillCompanies error = %v, want the reader error", err)
	}
}

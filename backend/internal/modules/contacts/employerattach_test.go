// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAContactCreateNamingBothAnIDAndANameIsRefused(t *testing.T) {
	_, err := ContactCreationInput(crmcontracts.CreateContactRequest{
		FullName: "Both Ways", Source: "manual",
		CompanyId: new(openapi_types.UUID(ids.NewV7())), CompanyName: new("Also New GmbH"),
	})
	refused, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || len(refused.Fields) != 1 || refused.Fields[0].Field != fieldCompanyName {
		t.Fatalf("err = %v, want a 422 naming company_name", err)
	}
}

func TestABlankCompanyNameNamesNoEmployer(t *testing.T) {
	in, err := ContactCreationInput(crmcontracts.CreateContactRequest{
		FullName: "No Employer", Source: "manual", CompanyName: new("   "),
	})
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}
	if in.Employer.named() {
		t.Error("a blank company name names an employer")
	}
}

func TestLinkingAnEmployerNeedsOne(t *testing.T) {
	for name, in := range map[string]LinkEmployerInput{
		endCompanyColumn: {},
		fieldCompanyName: {CompanyName: new(" ")},
	} {
		_, err := (&Store{}).LinkContactEmployer(t.Context(), ids.From[ids.ContactKind](ids.NewV7()), in)
		if required, ok := errors.AsType[*RequiredFieldError](err); !ok || required.Field != name {
			t.Errorf("err = %v, want %s required", err, name)
		}
	}
}

func TestALinkBatchIsRefusedBeforeAnyRead(t *testing.T) {
	repeated := ids.NewV7()
	tooMany := make([]ids.UUID, linkCandidateCap+1)
	for i := range tooMany {
		tooMany[i] = ids.NewV7()
	}
	for name, tc := range map[string]struct {
		batch []ids.UUID
		code  string
	}{
		"empty":    {nil, codeRequired},
		"too many": {tooMany, "too_many"},
		"repeated": {[]ids.UUID{repeated, repeated}, "duplicate"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (&Store{}).LinkCompanyContacts(t.Context(), ids.From[ids.CompanyKind](ids.NewV7()), tc.batch)
			if field, code := refusalOf(err); field != fieldContactIDs || code != tc.code {
				t.Errorf("refusal = %s/%s, want %s/%s", field, code, fieldContactIDs, tc.code)
			}
		})
	}
}

func refusalOf(err error) (field, code string) {
	if required, ok := errors.AsType[*RequiredFieldError](err); ok {
		field, code, _ = required.FieldFault()
		return field, code
	}
	if refused, ok := errors.AsType[*httperr.DetailedError](err); ok && len(refused.Fields) > 0 {
		return refused.Fields[0].Field, refused.Fields[0].Code
	}
	return "", ""
}

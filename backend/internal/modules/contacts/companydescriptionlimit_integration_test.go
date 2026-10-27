// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The company_description_length CHECK refuses a long description with an
// answer that names no field. Create and update refuse it first, naming it.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACompanyDescriptionPastTheLimitIsRefusedNamingTheField(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	atTheLimit := strings.Repeat("x", CompanyDescriptionMax)
	tooLong := atTheLimit + "x"

	if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Long Words GmbH", Source: "manual", Description: &tooLong}); !refusesDescription(err) {
		t.Fatalf("create answered %v, want a refusal naming description", err)
	}
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Fits Exactly GmbH", Source: "manual", Description: &atTheLimit})
	if err != nil {
		t.Fatalf("a description at the limit was refused: %v", err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{Description: &tooLong}); !refusesDescription(err) {
		t.Fatalf("update answered %v, want a refusal naming description", err)
	}
}

func refusesDescription(err error) bool {
	refusal, ok := errors.AsType[*CompanyDescriptionTooLongError](err)
	if !ok {
		return false
	}
	field, _, _ := refusal.FieldFault()
	return field == "description"
}

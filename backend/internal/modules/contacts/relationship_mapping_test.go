// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestRelationshipMappingRefusesUnknownEmploymentStatus(t *testing.T) {
	for _, value := range []string{"current", "former", "unknown", "invalid"} {
		createStatus := crmcontracts.CreateRelationshipRequestEmploymentStatus(value)
		updateStatus := crmcontracts.UpdateRelationshipRequestEmploymentStatus(value)
		_, createErr := relationshipCreateInput(crmcontracts.CreateRelationshipRequest{Source: "manual", EmploymentStatus: &createStatus})
		_, updateErr := relationshipUpdateInput(crmcontracts.UpdateRelationshipRequest{EmploymentStatus: &updateStatus}, nil, nil)
		for _, err := range []error{createErr, updateErr} {
			if value == "current" || value == "former" {
				if err != nil {
					t.Fatalf("%s refused: %v", value, err)
				}
			} else {
				fault, ok := errors.AsType[*EmploymentImportError](err)
				if !ok || fault.Field != "employment_status" {
					t.Fatalf("%s: want employment_status refusal, got %v", value, err)
				}
			}
		}
	}
	if _, err := relationshipCreateInput(crmcontracts.CreateRelationshipRequest{Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, err := relationshipUpdateInput(crmcontracts.UpdateRelationshipRequest{}, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEmploymentImportCorrectionRefusesAnUnknownAssertion(t *testing.T) {
	value := crmcontracts.EmploymentImportRequestEmploymentStatus("unknown")
	_, err := employmentImportAction(crmcontracts.EmploymentImportRequest{EmploymentStatus: &value})
	if err == nil {
		t.Fatal("an explicit unknown correction was accepted")
	}
}

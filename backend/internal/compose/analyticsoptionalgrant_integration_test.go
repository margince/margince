// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A grant that only narrows the analytics vocabulary is asked unrecorded, so
// tools/list keeps offering the schema to a role without it. Naming the field
// outright still asks it recorded, and refuses.
func TestAnOptionalAnalyticsGrantNarrowsTheSchemaWithoutHidingTheTool(t *testing.T) {
	specs := map[string]reportSpec{"projects-by-phase": projectsByPhaseSpec()}
	ctx, recorded := auth.RecordChecksForTest(seatWith("project", "deal"))

	schema := analyticsSchemaFrom(ctx, specs)
	if _, named := schema.Entities["projects-by-phase"].Fields[fieldCompanyID]; named {
		t.Errorf("a role without company and relationship reads can name %s", fieldCompanyID)
	}
	for _, optional := range []string{"company.read", "relationship.read"} {
		if slices.Contains(recorded().Checks, optional) {
			t.Errorf("deriving the schema recorded %s, which would hide the tool from a role without it", optional)
		}
	}

	err := requireVocabularyGrants(ctx, projectsByPhaseSpec(), []string{fieldCompanyID})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("naming %s without the grants answered %v, want a refusal", fieldCompanyID, err)
	}
	if !slices.Contains(recorded().Checks, "company.read") {
		t.Errorf("the refusing check was not recorded: %v", recorded().Checks)
	}

	wide := analyticsSchemaFrom(seatWith("project", "deal", "company", "relationship"), specs)
	if _, named := wide.Entities["projects-by-phase"].Fields[fieldCompanyID]; !named {
		t.Errorf("a role with both grants cannot name %s", fieldCompanyID)
	}
}

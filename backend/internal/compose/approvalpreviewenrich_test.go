// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func TestEnrichmentPreviewRejectsUnknownAndRetargetedArguments(t *testing.T) {
	target := ids.NewV7()
	for _, payload := range []map[string]any{
		{"depth": "unknown"},
		{"url": "not-a-url"},
		{"company_id": ids.NewV7().String()},
		{"operation": "unknown", "body": map[string]any{}},
	} {
		_, err := enrichmentCommand(crmcontracts.Approval{TargetEntityId: new(openapi_types.UUID(target)), TargetEntityType: new("company"), ProposedChange: &payload})
		if !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Fatalf("payload %+v: %v", payload, err)
		}
	}
}

func TestEnrichmentPreviewUsesTheRouteDepthForAnEmptyRESTBody(t *testing.T) {
	target := ids.NewV7()
	payload := map[string]any{"operation": "deepReadCompany", "body": nil}
	command, err := enrichmentCommand(crmcontracts.Approval{TargetEntityId: new(openapi_types.UUID(target)), TargetEntityType: new("company"), ProposedChange: &payload})
	if err != nil || command.Depth != agents.EnrichDepthSite || command.CompanyID != target {
		t.Fatalf("command: %+v, %v", command, err)
	}
}

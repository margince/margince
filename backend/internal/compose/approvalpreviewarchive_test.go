// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestArchivePreviewBindsEveryRegisteredRouteToItsStagedTarget(t *testing.T) {
	matched := 0
	for route, policy := range agentPolicies {
		if policy.Tool != "archive_record" {
			continue
		}
		matched++
		target := ids.NewV7()
		path := strings.TrimPrefix(strings.ReplaceAll(route, "{id}", target.String()), "DELETE ")
		payload := map[string]any{"operation": policy.Op, "path": path, "body": nil}
		approval := crmcontracts.Approval{TargetEntityType: new(string(policy.RecordType)), TargetEntityId: new(crmcontracts.Id(target)), ProposedChange: &payload}
		command, err := archivePreviewCommand(approval)
		if err != nil || command.ID != target || command.RecordType != string(policy.RecordType) {
			t.Errorf("%s: %+v / %v", route, command, err)
		}
		payload["path"] = strings.ReplaceAll(path, target.String(), ids.NewV7().String())
		if _, err := archivePreviewCommand(approval); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%s accepted another target: %v", route, err)
		}
	}
	if matched == 0 {
		t.Fatal("no registered archive routes were checked")
	}
}

func TestArchivePreviewRefusesACommandForAnotherRecord(t *testing.T) {
	target := ids.NewV7()
	for _, payload := range []map[string]any{
		{"record_type": "product", "id": ids.NewV7()},
		{"record_type": "offer_template", "id": target},
		{"operation": map[string]string{"kind": "archiveProduct"}, "path": "/v1/products/" + target.String()},
		{"operation": "updateProduct", "path": "/v1/products/" + target.String()},
	} {
		_, err := archivePreviewCommand(crmcontracts.Approval{TargetEntityType: new("product"), TargetEntityId: new(crmcontracts.Id(target)), ProposedChange: &payload})
		if err == nil {
			t.Fatalf("mismatched command admitted: %+v", payload)
		}
	}
}

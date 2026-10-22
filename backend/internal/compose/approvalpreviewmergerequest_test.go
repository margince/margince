// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMergePreviewNamesTheSamePairForRESTAndTools(t *testing.T) {
	source, survivor, other := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, kind := range []string{"contact", "company"} {
		for _, surface := range []string{"tool", "rest"} {
			for _, edit := range []string{"none", "survivor", "source", "kind", "route", "missing_pin"} {
				t.Run(kind+"/"+surface+"/"+edit, func(t *testing.T) {
					payload := map[string]any{"record_type": kind, "source_id": source.String(), "target_id": survivor.String()}
					switch edit {
					case "survivor":
						payload["target_id"] = other.String()
					case "source":
						payload["source_id"] = survivor.String()
					case "kind":
						payload["record_type"] = "lead"
					}
					if surface == "rest" {
						collection, operation := "contacts", "mergeContact"
						if kind == "company" {
							collection, operation = "companies", "mergeCompany"
						}
						path := "/v1/" + collection + "/" + source.String() + "/merge"
						if edit == "route" {
							path += "/other"
						}
						payload = map[string]any{"operation": operation, "path": path, "body": payload}
					}
					approval := crmcontracts.Approval{TargetEntityType: &kind, TargetEntityId: new(openapi_types.UUID(survivor)), TargetVersion: new(3), ProposedChange: &payload}
					if edit == "missing_pin" {
						approval.TargetVersion = nil
					}
					request, err := recordMergeRequest(approval)
					valid := edit == "none" || (edit == "route" && surface == "tool")
					if !valid {
						if !errors.Is(err, apperrors.ErrInvalidArgument) {
							t.Fatalf("unsafe merge accepted: request=%+v error=%v", request, err)
						}
						return
					}
					if err != nil || request.RecordType != kind || request.SourceID != source || request.TargetID != survivor {
						t.Fatalf("pair changed: request=%+v error=%v", request, err)
					}
				})
			}
		}
	}
}

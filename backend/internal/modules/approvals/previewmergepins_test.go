// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRecordMergePreviewBindsBothOperandsForEachTransport(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		for _, rest := range []bool{false, true} {
			for _, changed := range []string{"neither", "source", "survivor", "legacy"} {
				t.Run(fmt.Sprintf("%s/rest=%t/%s", kind, rest, changed), func(t *testing.T) {
					source, survivor := ids.NewV7(), ids.NewV7()
					subject := row{Kind: "merge_records", TargetType: &kind, TargetID: &survivor, TargetVersion: new(int64(2)), CoTargetType: &kind, CoTargetID: &source, CoTargetVersion: new(int64(1))}
					proposedSource, proposedSurvivor := source, survivor
					switch changed {
					case "source":
						proposedSource = ids.NewV7()
					case "survivor":
						proposedSurvivor = ids.NewV7()
					case "legacy":
						subject.CoTargetVersion = nil
					}
					payload := map[string]any{"record_type": kind, "source_id": proposedSource.String(), "target_id": proposedSurvivor.String()}
					if rest {
						operation, collection := "mergeContact", "contacts"
						if kind == "company" {
							operation, collection = "mergeCompany", "companies"
						}
						payload = map[string]any{"operation": operation, "path": "/v1/" + collection + "/" + proposedSource.String() + "/merge", "body": map[string]any{"target_id": proposedSurvivor.String()}}
					}
					if got := previewCoTargetPinned(subject, crmcontracts.Approval{ProposedChange: &payload}); got != (changed == "neither") {
						t.Fatalf("changed=%s pins match=%t", changed, got)
					}
				})
			}
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// The receipt lane makes a claim about WHO acted, and the claim is the whole
// point of the lane: "done for you" says the system handled something so the
// reader did not have to. Telling somebody that about a decision they made
// themselves is not a cosmetic error — it misreports their own work back to
// them, and it inflates what the product looks like it is doing.
//
// The test is decided_by IS NULL, the convention the expiry sweep states in its
// own words ("decided_by stays NULL and the actor is the system: nobody
// decided"). Status alone cannot tell the two apart: a human approval and a
// system one are both `approved`.
func TestAReceiptIsSomethingNobodyWasAskedAbout(t *testing.T) {
	decidedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	since := decidedAt.Add(-time.Hour)
	human := openapi_types.UUID(ids.NewV7())

	byTheSystem := approvalRow("Moved the Acme close date to 27 Sep", decidedAt, nil)
	byTheReader := approvalRow("Sent the Weber follow-up", decidedAt, &human)

	receipts := receiptsWithin([]crmcontracts.Approval{byTheSystem, byTheReader}, since)

	if len(receipts) != 1 {
		t.Fatalf("the lane carries %d receipts, want only the system's one", len(receipts))
	}
	if receipts[0].Summary != "Moved the Acme close date to 27 Sep" {
		t.Errorf("the lane kept %q — a reader's own decision is not something that ran for them", receipts[0].Summary)
	}
}

func approvalRow(summary string, decidedAt time.Time, decidedBy *openapi_types.UUID) crmcontracts.Approval {
	return crmcontracts.Approval{
		Id:        openapi_types.UUID(ids.NewV7()),
		Kind:      "close_date_correction",
		Summary:   &summary,
		DecidedAt: &decidedAt,
		DecidedBy: decidedBy,
	}
}

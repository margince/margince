// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCompanyMergePreviewMatchesReceiptRetentionAndHandoffMovement(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		name := "vacant"
		if occupied {
			name = "occupied"
		}
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			source, target := mergeContentPair(t, e, "company")
			sourceID, targetID := ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target)
			receipt := contacts.VatCheck{CompanyID: sourceID, Number: "DE123456789", Status: contacts.VatCheckValid, ConsultationNumber: "source-receipt", CheckedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
			if err := e.Contacts.RecordVatCheck(ctx, receipt); err != nil {
				t.Fatal(err)
			}
			if occupied {
				receipt.CompanyID, receipt.ConsultationNumber = targetID, "target-receipt"
				if err := e.Contacts.RecordVatCheck(ctx, receipt); err != nil {
					t.Fatal(err)
				}
			}
			contact, _ := mergeContentPair(t, e, "contact")
			handoff, err := e.Contacts.SubmitHandoff(ctx, contacts.NewSDRHandoff{ContactID: &contact, CompanyID: &source, Note: "Review account ownership"})
			if err != nil {
				t.Fatal(err)
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "company", source, target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.CompanySingletons) != 1 || preview.CompanySingletons[0].Kind != "vat_check" || preview.CompanySingletons[0].Retained != occupied {
				t.Fatalf("receipt preview=%+v", preview.CompanySingletons)
			}
			if len(preview.Handoffs) != 1 || preview.Handoffs[0].ID != handoff {
				t.Fatalf("company handoffs=%+v", preview.Handoffs)
			}
			if _, err := e.Contacts.MergeCompany(ctx, sourceID, targetID, &preview.SourceVersion); err != nil {
				t.Fatal(err)
			}
			expected := "source-receipt"
			if occupied {
				expected = "target-receipt"
			}
			actual, err := e.Contacts.VatCheckFor(ctx, targetID)
			if err != nil || actual.ConsultationNumber != expected {
				t.Fatalf("native receipt disagrees: %+v / %v", actual, err)
			}
			if actual := e.WsScalar(t, `SELECT company_id::text FROM sdr_handoff WHERE id=@id`, pgx.StrictNamedArgs{"id": handoff}); actual != target.String() {
				t.Fatalf("handoff moved to %s", actual)
			}
		})
	}
}

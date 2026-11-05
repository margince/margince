// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestImportPreviewNamesTheCompanyRowInsteadOfPromisingAnEmptyLink(t *testing.T) {
	links := []migration.ReviewLink{{Object: migration.ObjectCompany, Pending: true, PendingKey: "company-one"}}
	reader := approvalPreviewReader{}
	details, err := reader.importLinkDetails(context.Background(), links)
	if err != nil {
		t.Fatal(err)
	}
	details = append(omitPendingImportDefaults([]crmcontracts.ApprovalPreviewDetail{
		previewDetail("company_id", ""), previewDetail("name", "Expansion"),
	}, links), details...)
	if len(details) != 2 || details[0].Field != "name" || details[1].Field != "import_link_row_company" || details[1].Value.Text != "company-one" {
		t.Fatalf("creation shows contradictory or incomplete relationship: %+v", details)
	}
	row := crmcontracts.ApprovalPreviewImportRow{}
	if err := reader.importPendingCompanyChange(context.Background(), migration.ObjectDeal, []byte(`{"company_id":null}`), links, &row); err != nil {
		t.Fatal(err)
	}
	if len(row.Changes) != 1 || row.Changes[0].Before.Kind != previewValueEmpty || row.Changes[0].After.Text != "company-one" {
		t.Fatalf("existing deal lacks the exact pending company change: %+v", row.Changes)
	}
}

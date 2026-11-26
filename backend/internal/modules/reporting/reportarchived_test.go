// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestAnArchivedReportRefusesNewWorkAsAConflictWithItsOwnCode(t *testing.T) {
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	fault, ok := httperr.Classify(refuseArchived(crmcontracts.ReportingReport{ArchivedAt: &at}))
	if !ok || fault.Status != http.StatusConflict || fault.Code != codeReportArchived {
		t.Fatalf("want 409 %s, got %+v", codeReportArchived, fault)
	}
	if fault.Detail == "" {
		t.Fatal("the refusal names no reason")
	}
}

func TestALiveReportIsNotRefusedAsArchived(t *testing.T) {
	if err := refuseArchived(crmcontracts.ReportingReport{}); err != nil {
		t.Fatalf("a live report was refused: %v", err)
	}
}

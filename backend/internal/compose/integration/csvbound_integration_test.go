// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
)

// A ref one import mapped to a company stands for that company in every later
// slice and run. A row naming another company under it is refused with a reason,
// and the contact links to the first company, so the file never splits.
func TestARefMappedToAnotherCompanyIsNotRebound(t *testing.T) {
	e := setupImportApp(t)
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Other Works"})
	first := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,,Created Works,\n")
	approveAndRead(t, e, first.ID)

	second := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Other Works,\n")
	approveAndRead(t, e, second.ID)
	committed := readImportReport(t, e, second.ID)
	if committed.Disposition.Skipped != 1 {
		t.Fatalf("committed = %+v, want the company row skipped", committed)
	}
	if employer := employerOf(t, e, "ida@named.example"); employer != "Created Works" {
		t.Fatalf("employer = %q, want the company the ref stood for first", employer)
	}
}

// Undo frees a bound ref whatever its company is. The company was never the
// run's, so a live one must not keep answering for the ref.
func TestAnUndoneRunsBoundRefIsFreeToImportAgain(t *testing.T) {
	e := setupImportApp(t)
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	first := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Named Works,\n")
	approveAndRead(t, e, first.ID)
	if status := importCall(t, e, first.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d, want 202", status)
	}

	again := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,,Fresh Works,\n")
	predicted := readImportReport(t, e, again.ID)
	if predicted.Disposition.Created != 2 || predicted.Disposition.Updated != 0 {
		t.Fatalf("dry run = %+v, want the company created, not an update of the old one", predicted)
	}
	approveAndRead(t, e, again.ID)
	if employer := employerOf(t, e, "ida@named.example"); !strings.HasPrefix(employer, "Fresh") {
		t.Fatalf("employer = %q, want the company the new import created", employer)
	}
}

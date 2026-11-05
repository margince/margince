// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Keeping an undecided domain over the real wire. A domain nobody asked about
// is not found. A seat without the company grant is refused the same way
// whether or not the domain is open.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestKeepingADomainNobodyAskedAboutIsNotFound(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	if status := e.Call(t, "POST", "/v1/capture/domain-questions/never-seen.example/keep",
		nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("keep of an unknown domain → %d, want 404", status)
	}
}

func TestKeepingIsRefusedToASeatWithoutTheCompanyGrantWhetherOrNotTheDomainIsOpen(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO company_domain_disposition (domain, status, source)
		 VALUES ('open-question.example', 'pending', NULL)`); err != nil {
		t.Fatalf("opening a question: %v", err)
	}

	e.SetWorkspaceSeat(t, "read")

	for _, domain := range []string{"open-question.example", "never-seen.example"} {
		if status := e.Call(t, "POST", "/v1/capture/domain-questions/"+domain+"/keep",
			nil, nil, nil); status != http.StatusForbidden {
			t.Errorf("read-seat keep of %s → %d, want 403 either way", domain, status)
		}
	}
}

// The keep-vendor route is wired to its handler. A domain nobody asked about is
// not found. A seat without the company grant is refused before any row is read.
func TestKeepingAVendorVerdictOverTheWireAnswersOnlyAnAskedSeat(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if status := e.Call(t, "POST", "/v1/capture/domain-questions/never-seen.example/keep-vendor",
		nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("keep-vendor of an unknown domain → %d, want 404", status)
	}
	e.SetWorkspaceSeat(t, "read")
	if status := e.Call(t, "POST", "/v1/capture/domain-questions/never-seen.example/keep-vendor",
		nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("read-seat keep-vendor → %d, want 403", status)
	}
}

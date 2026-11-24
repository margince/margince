// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// footerWarning answers System health's footer finding for the default entity,
// nil when it raises none. A seat the page refuses answers nil too.
func footerWarning(t *testing.T, e *apptest.AppEnv) *crmcontracts.HealthFinding {
	t.Helper()
	var report crmcontracts.SystemHealth
	status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &report)
	if status == http.StatusForbidden {
		return nil
	}
	if status != http.StatusOK {
		t.Fatalf("system health = %d", status)
	}
	i := slices.IndexFunc(report.Findings, func(f crmcontracts.HealthFinding) bool {
		return f.Code == "footer_incomplete" && f.FooterGap != nil && f.FooterGap.IsDefault
	})
	if i < 0 {
		return nil
	}
	return &report.Findings[i]
}

// A rep's send held for a missing objection route warns the admins: it names
// the missing detail and counts the held send. The rep is not told. Stating the
// detail removes the warning.
//
// Mutation: drop RecordFooterHoldTx from consent.Gate.RecordRefusal and this
// fails.
func TestAHeldSendWarnsAdminsUntilTheCompanyDetailIsStated(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	if _, err := p.Owner.Exec(t.Context(), `UPDATE contact SET address_country=@country WHERE id=@contact`, pgx.StrictNamedArgs{"contact": p.contactID, "country": footerOnlyCountry()}); err != nil {
		t.Fatal(err)
	}
	objection := crmcontracts.HealthFooterGapMissing(messagingrules.ObjectionRoute)
	if warning := footerWarning(t, p.AppEnv); warning != nil && slices.Contains(warning.FooterGap.Missing, objection) {
		t.Fatalf("warned before any send owed the detail: %+v", warning.FooterGap)
	}
	demoteToRep(t, p.AppEnv)
	if status, _, _ := p.send(t); status != http.StatusConflict {
		t.Fatalf("footer-only send = %d, want it held for review", status)
	}
	if warning := footerWarning(t, p.AppEnv); warning != nil {
		t.Fatalf("a rep was shown the admins' footer warning: %+v", warning)
	}

	assignSessionRole(t, p.AppEnv, "admin")
	warning := footerWarning(t, p.AppEnv)
	if warning == nil || !slices.Contains(warning.FooterGap.Missing, objection) {
		t.Fatalf("no warning names the missing objection route: %+v", warning)
	}
	if warning.FooterGap.HeldByPerson != 1 || warning.FooterGap.HeldInBackground != 0 || !warning.RequiresAction {
		t.Errorf("warning %+v, want one held send by a person that needs an admin", warning.FooterGap)
	}
	if warning.Action.Destination != crmcontracts.HealthActionDestinationDisclosures {
		t.Errorf("the warning opens %s, want the disclosure page", warning.Action.Destination)
	}

	if status := p.Call(t, "PUT", "/v1/privacy/controller-particulars", AnyMap{"objection_route": "Reply STOP."}, nil, nil); status != http.StatusOK {
		t.Fatalf("stating the objection route = %d", status)
	}
	if warning := footerWarning(t, p.AppEnv); warning != nil && slices.Contains(warning.FooterGap.Missing, objection) {
		t.Errorf("the warning outlived the stated detail: %+v", warning.FooterGap)
	}
}

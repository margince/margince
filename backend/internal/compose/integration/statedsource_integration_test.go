// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// TestAHandCreatedContactsPrivacyPageNamesTheStatedSource. A user creating a
// contact by hand states its source. That source reaches the privacy page.
func TestAHandCreatedContactsPrivacyPageNamesTheStatedSource(t *testing.T) {
	t.Parallel()
	e := apptest.SetupAppWithOptions(t, compose.WithOperatorMail(apptest.DiscardingMailer{}))
	apptest.BootstrapWorkspaceSession(t, e, "Stated Source", "dpo@stated.test", "Admin")
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Card Holder",
		"emails":             []AnyMap{{"email": "card@stated.test"}},
		"acquisition_source": "business_card",
		"acquisition_note":   "Trade fair in Hanoi",
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("creating the contact → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/contacts/"+contact.ID+"/consent/privacy-notice",
		AnyMap{}, nil, nil); status >= 400 {
		t.Fatalf("sending the privacy notice → %d", status)
	}

	var page crmcontracts.PrivacyInformationPage
	if status := publicCall(t, e, "GET", "/v1/public/confirm/"+confirmLinkToken(t, e), nil, nil, &page); status != http.StatusOK {
		t.Fatalf("opening the privacy page → %d", status)
	}
	if page.StatedSource == nil || *page.StatedSource != crmcontracts.ContactStatedSourceBusinessCard ||
		page.StatedSourceNote == nil || *page.StatedSourceNote != "Trade fair in Hanoi" {
		t.Errorf("the page states source %v / %v, want business_card / the note", page.StatedSource, page.StatedSourceNote)
	}
}

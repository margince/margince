// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TestThePrivacyPageShowsWhatThePrivacyModuleHoldsForTheReader opens a notice
// link through the public edge and the handlers the server assembles. The page
// must show the reader's own kinds and the retention rules that reach them.
func TestThePrivacyPageShowsWhatThePrivacyModuleHoldsForTheReader(t *testing.T) {
	l := setupConfirmLane(t)
	rules := privacy.NewPolicyStore(InstallationDB(l.e.Pool))
	author := l.e.As(l.e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"retention_policy": {Create: true, Read: true}},
		RowScope: principal.RowScopeAll,
	})
	for _, policy := range []privacy.PolicyInput{
		{Scope: privacy.RetentionScope{ObjectType: "activity", Category: "transcript"}, RetainDays: 45, Action: "archive", Enabled: true},
		{Scope: privacy.RetentionScope{ObjectType: "contact", Category: "no_consent_no_deal"}, RetainDays: 700, Action: "anonymize", Enabled: true},
	} {
		if _, err := rules.Create(author, policy); err != nil {
			t.Fatalf("authoring %s: %v", policy.Scope, err)
		}
	}
	if _, err := l.store.IssuePrivacyNotice(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("issuing the notice: %v", err)
	}
	token, _ := sealedConfirmToken(l.vault.sealed)

	h := newConsentHandlers(l.e.Pool)
	page := publicConfirm(newPublicConfirmLimiters())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.GetConfirmDetails(w, r, token)
	}))
	rec := httptest.NewRecorder()
	page.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, publicConfirmPrefix+token, nil).WithContext(l.ctx))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var notice crmcontracts.PrivacyInformationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &notice); err != nil {
		t.Fatal(err)
	}
	if len(notice.Retention) != 1 || notice.Retention[0].Scope != "contact/no_consent_no_deal" ||
		notice.Retention[0].RetainDays != 700 {
		t.Errorf("the page shows retention %+v, want the record rule alone: the reader has no transcript", notice.Retention)
	}
	if !slices.Contains(notice.DataCategories, "email") {
		t.Errorf("the page names kinds %v, want the reader's address among them", notice.DataCategories)
	}
}

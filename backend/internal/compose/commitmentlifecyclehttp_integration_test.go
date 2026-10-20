// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func settleClaimHTTP(t *testing.T, e *transcriptEnv, claim ids.UUID, outcome string) {
	t.Helper()
	body, err := json.Marshal(crmcontracts.SettleClaimRequest{Outcome: crmcontracts.SettleClaimRequestOutcome(outcome)})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/claims/"+claim.String()+"/settle", bytes.NewReader(body)).WithContext(e.Admin())
	contacts.NewHandlers(e.DB()).SettleConversationClaim(response, request, crmcontracts.Id(claim))
	if response.Code != http.StatusNoContent {
		t.Fatalf("claim settlement HTTP %d: %s", response.Code, response.Body.String())
	}
}

func assertPromiseProjectionHTTP(t *testing.T, e *transcriptEnv, approval ids.UUID, want int) {
	t.Helper()
	authority, _, err := identity.NewService(e.Pool).EffectiveAuthority(e.Admin(), e.WS, e.Rep2)
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.As(e.Rep2, authority.TeamIDs, authority.Permissions)
	clock := func() time.Time { return completionReadTime }
	home := httptest.NewRecorder()
	magic.NewHandlers(newMagicService(e.Pool, e.svc, clock)).GetMagic(home, httptest.NewRequest(http.MethodGet, "/v1/magic", nil).WithContext(ctx), crmcontracts.GetMagicParams{})
	if home.Code != http.StatusOK {
		t.Fatalf("Home HTTP %d: %s", home.Code, home.Body.String())
	}
	var receipt crmcontracts.MagicReceipt
	if err := json.Unmarshal(home.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, row := range receipt.NeedsYou {
		if ids.UUID(row.Id) == approval {
			found++
		}
	}
	if found != want {
		t.Fatalf("Home has %d matching decisions, want %d", found, want)
	}
	feed := newAttentionService(e.Pool, e.svc, clock)
	response := httptest.NewRecorder()
	attention.NewHandlers(feed).GetAttention(response, httptest.NewRequest(http.MethodGet, "/v1/attention", nil).WithContext(ctx))
	if response.Code != http.StatusOK {
		t.Fatalf("attention HTTP %d: %s", response.Code, response.Body.String())
	}
	var day crmcontracts.Attention
	if err := json.Unmarshal(response.Body.Bytes(), &day); err != nil {
		t.Fatal(err)
	}
	if day.Counts.NeedsYou != want || day.Counts.Notices == nil || *day.Counts.Notices != 0 {
		t.Fatalf("duplicate day counts: %+v", day.Counts)
	}
	if day.NoticesMoreAvailable == nil || *day.NoticesMoreAvailable {
		t.Fatal("the complete notice read lost its coverage through HTTP")
	}
	params := crmcontracts.GetWorklistParams{Limit: new(1)}
	found = 0
	for range 20 {
		page := readWorklistHTTP(ctx, t, feed, params)
		for _, row := range page.Queue {
			if row.Source == "notice" {
				t.Fatal("decision announcement became a second queue item")
			}
			if row.Id == approval.String() {
				found++
			}
		}
		if page.Focus != nil {
			for _, row := range page.Focus.Items {
				if row.Source == "notice" {
					t.Fatal("announcement became a second focus item")
				}
			}
		}
		if page.NextCursor == nil {
			if found != want {
				t.Fatalf("Worklist has %d matching decisions, want %d", found, want)
			}
			return
		}
		params.Cursor = page.NextCursor
	}
	t.Fatal("Worklist continuation did not finish")
}

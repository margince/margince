// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The reach ranking pages to its tail. Most accounts share a connection count,
// so the walk must resume inside a tie, and a cursor it did not mint is refused.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reachNetwork is how many exported connections work at each account.
var reachNetwork = map[string]int{
	"Alder Logistik":  3,
	"Birch Maschinen": 2, "Cedar Handel": 2, "Dogwood Bau": 2, "Elm Druck": 2, "Fir Textil": 2,
}

// twinName is what three tied accounts are renamed to after placement. They
// then tie on name as well, and only the id orders them.
const twinName = "Twin Holding"

// seedReachNetwork creates the accounts, imports a network reaching them and
// places it, all through the store's own writers.
func (e *dedupeEnv) seedReachNetwork(t *testing.T) map[ids.UUID]string {
	t.Helper()
	ctx := e.as()
	var export strings.Builder
	export.WriteString("First Name,Last Name,URL,Email Address,Company,Position,Connected On\n")
	byID := map[ids.UUID]string{}
	for name, connections := range reachNetwork {
		byID[e.createNamedCompany(ctx, t, name).UUID] = name
		for n := range connections {
			slug := strings.ToLower(strings.ReplaceAll(name, " ", "")) + fmt.Sprint(n)
			fmt.Fprintf(&export, "Reach,%s,https://www.linkedin.com/in/%s,,%s,Buyer,01 Jan 2024\n", slug, slug, name)
		}
	}
	if _, err := e.store.ImportLinkedInConnections(ctx, strings.NewReader(export.String())); err != nil {
		t.Fatalf("importing the network: %v", err)
	}
	if _, err := e.store.MatchLinkedInConnections(ctx, e.rep); err != nil {
		t.Fatalf("placing the network: %v", err)
	}
	renamed := 0
	for id, name := range byID {
		if reachNetwork[name] != 2 || renamed == 3 {
			continue
		}
		if _, err := e.store.UpdateCompany(ctx, ids.From[ids.CompanyKind](id),
			UpdateCompanyInput{DisplayName: new(twinName)}); err != nil {
			t.Fatalf("renaming %s: %v", name, err)
		}
		renamed++
	}
	return byID
}

// readReachPage asks the handler for one page, as a client would.
func (e *dedupeEnv) readReachPage(t *testing.T, limit int, cursor *string) (int, crmcontracts.LinkedInReachResponse, string) {
	t.Helper()
	query := url.Values{"limit": {fmt.Sprint(limit)}}
	if cursor != nil {
		query.Set("cursor", *cursor)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(e.as(), http.MethodGet, "/v1/me/linkedin-reach?"+query.Encode(), nil)
	Handlers{store: e.store}.GetMyLinkedInReach(rec, req,
		crmcontracts.GetMyLinkedInReachParams{Limit: &limit, Cursor: cursor})
	var body crmcontracts.LinkedInReachResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decoding the reach page: %v", err)
		}
	}
	return rec.Code, body, rec.Body.String()
}

func TestTheReachWalkReturnsEveryTiedAccountOnce(t *testing.T) {
	e := setupDedupe(t)
	placed := e.seedReachNetwork(t)

	seen := map[ids.UUID]int{}
	var cursor *string
	previous := 0
	for pages := 0; ; pages++ {
		if pages > len(placed) {
			t.Fatalf("the walk is still going after %d pages of %d accounts", pages, len(placed))
		}
		code, page, raw := e.readReachPage(t, 2, cursor)
		if code != http.StatusOK {
			t.Fatalf("page %d answered %d: %s", pages, code, raw)
		}
		// The total counts the whole network on every page, not the page.
		if page.AccountsTotal != len(placed) {
			t.Errorf("page %d reports %d accounts in total, want %d", pages, page.AccountsTotal, len(placed))
		}
		for _, account := range page.Accounts {
			if previous > 0 && account.Connections > previous {
				t.Errorf("%s (%d) ranks below an account with %d", account.DisplayName, account.Connections, previous)
			}
			previous = account.Connections
			seen[ids.UUID(account.CompanyId)]++
		}
		if page.NextCursor == nil {
			break
		}
		cursor = page.NextCursor
	}
	for id, name := range placed {
		if seen[id] != 1 {
			t.Errorf("%s came back %d times across the walk, want once", name, seen[id])
		}
	}
	if len(seen) != len(placed) {
		t.Errorf("the walk returned %d distinct accounts, want %d", len(seen), len(placed))
	}
}

func TestAReachCursorFromAnotherWalkIsRefused(t *testing.T) {
	e := setupDedupe(t)
	e.seedReachNetwork(t)
	// The webhook history's position: well-formed, ours, and no place in this
	// ranking. Answered as a page, it would read as the end of the network.
	foreign, err := storekit.EncodeOpaque(struct {
		At time.Time `json:"at"`
		ID ids.UUID  `json:"id"`
	}{At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), ID: ids.NewV7()})
	if err != nil {
		t.Fatalf("minting the foreign cursor: %v", err)
	}
	code, _, raw := e.readReachPage(t, 2, &foreign)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("a foreign cursor answered %d %s, want 422", code, raw)
	}
}

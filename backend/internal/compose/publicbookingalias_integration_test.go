// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// aliasHosts gives the admin ("Ada") and Rep1 ("Ben") a live booking page on
// one shared clock, so a test can move past the alias cooldown.
type aliasHosts struct {
	e        *integration.Env
	store    *activities.Store
	now      time.Time
	ada, ben context.Context
	server   http.Handler
}

func newAliasHosts(t *testing.T) *aliasHosts {
	t.Helper()
	e := integration.Setup(t)
	h := &aliasHosts{e: e, now: clocktest.Now(t)}
	h.store = e.Activities.WithClock(func() time.Time { return h.now }).WithPublicBaseURL(previewOrigin).
		WithSchedulingCalendar(privateCalendar{}).WithSchedulingBrand(e.Contacts)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Ada Host' WHERE id = $1`, e.AdminUser)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Ben Host', status = 'active' WHERE id = $1`, e.Rep1)
	h.ada, h.ben = e.Admin(), e.As(e.Rep1, nil, integration.AdminPerms)
	for _, ctx := range []context.Context{h.ada, h.ben} {
		if _, err := h.store.SaveSchedulingProfile(ctx, crmcontracts.SchedulingProfile{
			Enabled: true, Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Intro",
		}); err != nil {
			t.Fatal(err)
		}
	}
	h.server = New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), WithPublicBaseURL(previewOrigin))
	return h
}

func (h *aliasHosts) claim(t *testing.T, ctx context.Context, alias string) crmcontracts.SchedulingProfile {
	t.Helper()
	profile, err := h.store.SetSchedulingAlias(ctx, alias)
	if err != nil {
		t.Fatalf("claiming %q: %v", alias, err)
	}
	return profile
}

func (h *aliasHosts) refused(t *testing.T, ctx context.Context, alias, code string) {
	t.Helper()
	_, err := h.store.SetSchedulingAlias(ctx, alias)
	var refusal *activities.SchedulingArgumentError
	if !errors.As(err, &refusal) || refusal.Field != "alias" || refusal.Code != code {
		t.Fatalf("claiming %q: %v, want %s", alias, err, code)
	}
}

// host answers whose public profile an address opens, or "" for a 404.
func (h *aliasHosts) host(t *testing.T, address string) string {
	t.Helper()
	response := httptest.NewRecorder()
	h.server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/public/booking/"+address+"/profile", nil))
	if response.Code == http.StatusNotFound {
		return ""
	}
	var profile crmcontracts.PublicSchedulingProfile
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &profile) != nil {
		t.Fatalf("%s: %d %s", address, response.Code, response.Body.String())
	}
	return profile.HostName
}

func TestABookingAliasOpensTheSamePageAsTheSlugAndBecomesTheSharedAddress(t *testing.T) {
	h := newAliasHosts(t)
	before, err := h.store.SchedulingProfile(h.ada)
	if err != nil {
		t.Fatal(err)
	}
	profile := h.claim(t, h.ada, "  Ada-Sales ")
	if profile.Alias == nil || *profile.Alias != "ada-sales" || profile.PublicUrl == nil || *profile.PublicUrl != previewOrigin+"/book/ada-sales" {
		t.Fatalf("profile: alias=%v url=%v", profile.Alias, profile.PublicUrl)
	}
	for _, address := range []string{"ada-sales", "ADA-SALES", "Ada-Sales", *before.Slug} {
		if got := h.host(t, address); got != "Ada Host" {
			t.Fatalf("%s opened %q", address, got)
		}
	}
	// The random slug stays case-sensitive; normalization never reaches it.
	if got := h.host(t, strings.ToUpper(*before.Slug)); got != "" && *before.Slug != strings.ToUpper(*before.Slug) {
		t.Fatalf("an upper-cased slug opened %q", got)
	}
	// The preview card's canonical address is the alias, whichever address was fetched.
	card := fetchBookingPreview(t, h.server, http.MethodGet, *before.Slug).Body.String()
	if !strings.Contains(card, `<link rel="canonical" href="`+previewOrigin+`/book/ada-sales">`) {
		t.Fatalf("canonical:\n%s", card)
	}
	// Audited on the booking page, with an outbox event.
	if n := h.e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON o.envelope->'trace'->>'audit_log_id' = a.id::text
   WHERE a.entity_type = 'booking_page' AND a.after->>'alias' = 'ada-sales'`); n != 1 {
		t.Fatalf("audited alias claims with an event: %d", n)
	}
	// Claiming the same alias again changes nothing.
	h.claim(t, h.ada, "ada-sales")
	if n := h.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'booking_page' AND after->>'alias' = 'ada-sales'`); n != 1 {
		t.Fatalf("a repeated claim wrote again: %d", n)
	}
}

func TestAnAliasIsRefusedWithAFieldErrorWhenInvalidReservedOrTaken(t *testing.T) {
	h := newAliasHosts(t)
	h.claim(t, h.ada, "ada")
	h.refused(t, h.ben, "ADA", "taken")
	h.refused(t, h.ben, "ad", "too_short")
	h.refused(t, h.ben, "ben.host", "invalid")
	h.refused(t, h.ben, "manage-ben", "reserved")
	h.refused(t, h.ben, "preview", "reserved")
	ben, err := h.store.SchedulingProfile(h.ben)
	if err != nil {
		t.Fatal(err)
	}
	if ben.Alias != nil {
		t.Fatalf("a refused claim left an alias: %v", *ben.Alias)
	}
}

func TestARetiredAliasNeverSendsGuestsToAnotherHostBeforeItsCooldown(t *testing.T) {
	h := newAliasHosts(t)
	h.claim(t, h.ada, "team")
	h.claim(t, h.ada, "ada") // rename: "team" is retired
	if got := h.host(t, "team"); got != "" {
		t.Fatalf("a renamed alias still opens %q", got)
	}
	h.refused(t, h.ben, "team", "recently_used")
	h.now = h.now.Add(89 * 24 * time.Hour)
	h.refused(t, h.ben, "team", "recently_used")
	if got := h.host(t, "team"); got != "" {
		t.Fatalf("a retired alias opens %q", got)
	}
	h.now = h.now.Add(2 * 24 * time.Hour)
	h.claim(t, h.ben, "team")
	if got := h.host(t, "team"); got != "Ben Host" {
		t.Fatalf("after the cooldown team opens %q", got)
	}
	if got := h.host(t, "ada"); got != "Ada Host" {
		t.Fatalf("ada opens %q", got)
	}
}

func TestAHostMayTakeBackTheirOwnRetiredAliasAtOnce(t *testing.T) {
	h := newAliasHosts(t)
	h.claim(t, h.ada, "ada")
	h.claim(t, h.ada, "") // clear
	if got := h.host(t, "ada"); got != "" {
		t.Fatalf("a cleared alias opens %q", got)
	}
	profile, err := h.store.SchedulingProfile(h.ada)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Alias != nil || profile.PublicUrl == nil || *profile.PublicUrl != previewOrigin+"/book/"+*profile.Slug {
		t.Fatalf("after clearing: alias=%v url=%v", profile.Alias, profile.PublicUrl)
	}
	h.claim(t, h.ada, "ada")
	if got := h.host(t, "ada"); got != "Ada Host" {
		t.Fatalf("taken back, ada opens %q", got)
	}
}

func TestReplaceAddressRetiresTheAliasWithTheOldSlug(t *testing.T) {
	h := newAliasHosts(t)
	before := h.claim(t, h.ada, "ada")
	replace := true
	before.ReplaceLink = &replace
	after, err := h.store.SaveSchedulingProfile(h.ada, before)
	if err != nil {
		t.Fatal(err)
	}
	if after.Alias != nil || *after.Slug == *before.Slug || *after.PublicUrl != previewOrigin+"/book/"+*after.Slug {
		t.Fatalf("after Replace address: alias=%v slug=%v url=%v", after.Alias, *after.Slug, *after.PublicUrl)
	}
	for _, address := range []string{"ada", *before.Slug} {
		if got := h.host(t, address); got != "" {
			t.Fatalf("%s still opens %q after Replace address", address, got)
		}
	}
	h.refused(t, h.ben, "ada", "recently_used")
	// The host may choose it again for the new page.
	h.claim(t, h.ada, "ada")
	if got := h.host(t, "ada"); got != "Ada Host" {
		t.Fatalf("re-chosen alias opens %q", got)
	}
}

func TestADeactivatedHostsAliasOpensNothingAndStaysTheirs(t *testing.T) {
	h := newAliasHosts(t)
	h.claim(t, h.ada, "ada")
	h.e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, h.e.AdminUser)
	if got := h.host(t, "ada"); got != "" {
		t.Fatalf("a deactivated host's alias opens %q", got)
	}
	if code := fetchBookingPreview(t, h.server, http.MethodGet, "ada").Code; code != http.StatusNotFound {
		t.Fatalf("preview: %d", code)
	}
	h.now = h.now.Add(365 * 24 * time.Hour)
	h.refused(t, h.ben, "ada", "taken")
}

func TestTwoHostsClaimingOneAliasAtOnceGetOneWinnerAndOneFieldError(t *testing.T) {
	h := newAliasHosts(t)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, ctx := range []context.Context{h.ada, h.ben} {
		wg.Go(func() {
			_, results[i] = h.store.SetSchedulingAlias(ctx, "sales")
		})
	}
	wg.Wait()
	won := 0
	for _, err := range results {
		var refusal *activities.SchedulingArgumentError
		switch {
		case err == nil:
			won++
		case errors.As(err, &refusal) && refusal.Code == "taken":
		default:
			t.Fatalf("unexpected result: %v", err)
		}
	}
	if won != 1 || h.e.WsCount(t, `SELECT count(*) FROM booking_alias WHERE alias = 'sales'`) != 1 {
		t.Fatalf("winners: %d", won)
	}
}

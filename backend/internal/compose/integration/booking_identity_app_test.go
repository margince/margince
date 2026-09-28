// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestBookingPageUsesTheCurrentAccountName(t *testing.T) {
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	if status := e.Call(t, "PUT", "/v1/me/display-name", AnyMap{"display_name": "Account name"}, nil, nil); status != http.StatusOK {
		t.Fatalf("name save: %d", status)
	}
	var profile crmcontracts.SchedulingProfile
	if status := e.Call(t, "GET", "/v1/scheduling/profile", nil, nil, &profile); status != http.StatusOK || profile.HostName == nil || *profile.HostName != "Account name" {
		t.Fatalf("initial profile: %d %+v", status, profile)
	}
	enableBookingPage(t, e)
	if status := e.Call(t, "GET", "/v1/scheduling/profile", nil, nil, &profile); status != http.StatusOK || profile.HostName == nil || *profile.HostName != "Account name" || !profile.Enabled || profile.PublicUrl == nil {
		t.Fatalf("enabled profile: %d %+v", status, profile)
	}
	if profile.Slug == nil {
		t.Fatal("enabled page has no slug")
	}
	for _, tc := range []struct{ name, expected string }{
		{"Renamed account", "Renamed account"},
		{strings.Repeat("名", 255), strings.Repeat("名", 199) + "…"},
	} {
		if status := e.Call(t, "PUT", "/v1/me/display-name", AnyMap{"display_name": tc.name}, nil, nil); status != http.StatusOK {
			t.Fatalf("rename: %d", status)
		}
		var public crmcontracts.PublicSchedulingProfile
		if status := publicCall(t, e, "GET", "/v1/public/booking/"+*profile.Slug+"/profile", nil, nil, &public); status != http.StatusOK || public.HostName != tc.expected {
			t.Fatalf("public profile: %d %+v", status, public)
		}
		profile.HostName = nil
		if status := e.Call(t, "PUT", "/v1/scheduling/profile", profile, nil, &profile); status != http.StatusOK || profile.HostName == nil || *profile.HostName != tc.expected {
			t.Fatalf("resave: %d %+v", status, profile)
		}
	}
}

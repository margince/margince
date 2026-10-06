// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestBookingProfileReadsAccountNameBeforeItsLinkExists(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.test")
	profile, err := store.SchedulingProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.HostName == nil || *profile.HostName != "Rep" || profile.Slug != nil || profile.PublicUrl != nil {
		t.Fatalf("uncreated profile: %+v", profile)
	}
	legacyName := "Unrelated name"
	profile.HostName = &legacyName
	saved, err := store.SaveSchedulingProfile(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if saved.HostName == nil || *saved.HostName != "Rep" || saved.Slug == nil || saved.PublicUrl == nil || saved.Enabled {
		t.Fatalf("saved profile: %+v", saved)
	}
	var storedName bool
	args := schedulingArgs{}
	err = e.owner.QueryRow(ctx, `SELECT scheduling_policy ? 'host_name' FROM booking_page WHERE host_user_id=`+args.add(e.rep), args...).Scan(&storedName)
	if err != nil {
		t.Fatal(err)
	}
	if storedName {
		t.Fatal("booking policy duplicated the account name")
	}
}

func TestBookingProfileIgnoresALegacyStoredName(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.test")
	profile, err := store.SaveSchedulingProfile(ctx, defaultSchedulingProfile())
	if err != nil {
		t.Fatal(err)
	}
	// No current writer persists this field; reproduce a previously saved policy.
	args := schedulingArgs{}
	_, err = e.owner.Exec(ctx, `UPDATE booking_page SET scheduling_policy = scheduling_policy || '{"host_name":"Obsolete name"}'::jsonb WHERE slug=`+args.add(*profile.Slug), args...)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = store.SchedulingProfile(ctx)
	if err != nil || profile.HostName == nil || *profile.HostName != "Rep" {
		t.Fatalf("legacy profile: %+v, %v", profile, err)
	}
}

// A host who picks a provider before connecting it gets the sentence the
// no-provider case already gives, naming the provider field, not a server error.
func TestEnablingBookingsWithoutAConnectedCalendarNamesTheProvider(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil).WithPublicBaseURL("https://crm.example.test").
		WithSchedulingCalendar(&invitationCalendar{checkErr: connector.ErrAuthRejected})
	profile := defaultSchedulingProfile()
	profile.Enabled = true
	profile.Provider = "gcal"

	_, err := store.SaveSchedulingProfile(ctx, profile)

	var refusal *SchedulingArgumentError
	if !errors.As(err, &refusal) || refusal.Field != "provider" || refusal.Code != "required" {
		t.Fatalf("an unconnected calendar answered %v, want a refusal naming provider", err)
	}
}

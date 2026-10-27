// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A proposal offering only a time that has arrived is refused by name.
//
// The expiry is the last option's start when that falls inside the seven-day
// bound, and a host may require zero notice. So an option starting at the
// instant of the request makes the link expire as it is created.
//
// The column refuses that, and a host who hears a constraint violation learns
// nothing.

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAProposalWhoseOnlyTimeHasArrivedIsRefusedByName(t *testing.T) {
	f := newInvitationFixture(t)
	profile := defaultSchedulingProfile()
	profile.Provider = "gcal"
	profile.BufferMinutes = 0
	// Zero notice is what the contract admits (notice_minutes minimum 0) and
	// what makes a start equal to now acceptable to the policy check.
	profile.NoticeMinutes = 0
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatalf("saving a zero-notice profile: %v", err)
	}
	// The option starts at the instant the request is made, so the derived
	// expiry equals the creation instant.
	f.now = monday(9)
	in := crmcontracts.MeetingProposalRequest{
		ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail,
		Subject: "Right now", DurationMinutes: 60,
	}
	in.Options = append(in.Options, struct {
		End   time.Time `json:"end"`
		Start time.Time `json:"start"`
	}{Start: monday(9), End: monday(10)})

	_, err := f.store.CreateProposal(f.ctx, in)
	var refusal *SchedulingArgumentError
	if !errors.As(err, &refusal) {
		t.Fatalf("creating a proposal with no usable lifetime answered %v, want a named refusal", err)
	}
	if refusal.Code != "no_usable_lifetime" {
		t.Errorf("refusal code = %q, want no_usable_lifetime", refusal.Code)
	}
}

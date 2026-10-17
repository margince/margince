// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contract bounds these texts in characters. Each field is probed alone, so
// reverting any one of them to a byte count fails here.
func TestSchedulingTextLimitsCountCharactersNotBytes(t *testing.T) {
	zeroWidthSpace := string(rune(0x200B))
	profile := func(change func(*crmcontracts.SchedulingProfile)) error {
		p := defaultSchedulingProfile()
		change(&p)
		return validateSchedulingLimits(p)
	}
	invite := func(change func(*crmcontracts.MeetingInvitationRequest)) error {
		in := crmcontracts.MeetingInvitationRequest{
			ContactId: openapi_types.UUID(ids.NewV7()), AttendeeEmail: "guest@example.test", Subject: "Discovery",
		}
		change(&in)
		return validateInvitation(in)
	}
	fits := func(n int) string { return strings.Repeat("é", n) }

	if err := profile(func(p *crmcontracts.SchedulingProfile) { p.Title = fits(200) }); err != nil {
		t.Errorf("profile title at its limit refused: %v", err)
	}
	if err := profile(func(p *crmcontracts.SchedulingProfile) { p.Location = fits(1000) }); err != nil {
		t.Errorf("profile location at its limit refused: %v", err)
	}
	if err := profile(func(p *crmcontracts.SchedulingProfile) { p.Title = fits(201) }); err == nil {
		t.Error("a 201-character profile title was accepted")
	}
	if err := profile(func(p *crmcontracts.SchedulingProfile) { p.Location = fits(1001) }); err == nil {
		t.Error("a 1001-character profile location was accepted")
	}
	if err := profile(func(p *crmcontracts.SchedulingProfile) { p.Title = zeroWidthSpace }); err == nil {
		t.Error("an invisible profile title was accepted")
	}

	if err := invite(func(in *crmcontracts.MeetingInvitationRequest) { in.Subject = fits(200) }); err != nil {
		t.Errorf("invitation subject at its limit refused: %v", err)
	}
	if err := invite(func(in *crmcontracts.MeetingInvitationRequest) { in.Description = fits(5000) }); err != nil {
		t.Errorf("invitation description at its limit refused: %v", err)
	}
	if err := invite(func(in *crmcontracts.MeetingInvitationRequest) { in.Location = fits(1000) }); err != nil {
		t.Errorf("invitation location at its limit refused: %v", err)
	}
	if err := invite(func(in *crmcontracts.MeetingInvitationRequest) { in.Subject = fits(201) }); err == nil {
		t.Error("a 201-character invitation subject was accepted")
	}
	if err := invite(func(in *crmcontracts.MeetingInvitationRequest) { in.Description = fits(5001) }); err == nil {
		t.Error("a 5001-character invitation description was accepted")
	}
	if err := invite(func(in *crmcontracts.MeetingInvitationRequest) { in.Location = fits(1001) }); err == nil {
		t.Error("a 1001-character invitation location was accepted")
	}
	if err := invite(func(in *crmcontracts.MeetingInvitationRequest) { in.Subject = zeroWidthSpace }); err == nil {
		t.Error("an invisible invitation subject was accepted")
	}
}

// Each limit refuses in its own words and names the field to fix,
// so the caller is told which setting to correct.
func TestEachSchedulingLimitNamesItsOwnField(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(*crmcontracts.SchedulingProfile)
		field  string
		code   string
	}{
		"duration low":     {func(p *crmcontracts.SchedulingProfile) { p.DurationMinutes = 14 }, "duration_minutes", "out_of_range"},
		"duration high":    {func(p *crmcontracts.SchedulingProfile) { p.DurationMinutes = 481 }, "duration_minutes", "out_of_range"},
		"notice negative":  {func(p *crmcontracts.SchedulingProfile) { p.NoticeMinutes = -1 }, "notice_minutes", "out_of_range"},
		"notice high":      {func(p *crmcontracts.SchedulingProfile) { p.NoticeMinutes = 10081 }, "notice_minutes", "out_of_range"},
		"horizon zero":     {func(p *crmcontracts.SchedulingProfile) { p.HorizonDays = 0 }, "horizon_days", "out_of_range"},
		"horizon high":     {func(p *crmcontracts.SchedulingProfile) { p.HorizonDays = 91 }, "horizon_days", "out_of_range"},
		"buffer negative":  {func(p *crmcontracts.SchedulingProfile) { p.BufferMinutes = -5 }, "buffer_minutes", "out_of_range"},
		"buffer high":      {func(p *crmcontracts.SchedulingProfile) { p.BufferMinutes = 121 }, "buffer_minutes", "out_of_range"},
		"blank title":      {func(p *crmcontracts.SchedulingProfile) { p.Title = "  " }, "title", "required"},
		"long title":       {func(p *crmcontracts.SchedulingProfile) { p.Title = strings.Repeat("t", 201) }, "title", "too_long"},
		"long location":    {func(p *crmcontracts.SchedulingProfile) { p.Location = strings.Repeat("l", 1001) }, "location", "too_long"},
		"missing calendar": {func(p *crmcontracts.SchedulingProfile) { p.CalendarId = "" }, "calendar_id", "required"},
		"long calendar id": {func(p *crmcontracts.SchedulingProfile) { p.CalendarId = strings.Repeat("c", 513) }, "calendar_id", "too_long"},
	} {
		t.Run(name, func(t *testing.T) {
			p := defaultSchedulingProfile()
			tc.change(&p)
			var refusal *SchedulingArgumentError
			if err := validateSchedulingLimits(p); !errors.As(err, &refusal) || refusal.Field != tc.field || refusal.Code != tc.code {
				t.Fatalf("err = %v, want a %s refusal on %s", err, tc.code, tc.field)
			}
		})
	}
	p := defaultSchedulingProfile()
	p.CalendarId = strings.Repeat("c", 512)
	if err := validateSchedulingLimits(p); err != nil {
		t.Errorf("a calendar id at its limit was refused: %v", err)
	}
}

func TestABlockingCalendarOverItsLimitIsRefusedByName(t *testing.T) {
	long := []string{strings.Repeat("c", 513)}
	p := defaultSchedulingProfile()
	p.Provider, p.BlockingCalendars = "gcal", &long
	var refusal *SchedulingArgumentError
	if err := validateSchedulingProfile(p); !errors.As(err, &refusal) || refusal.Field != "blocking_calendars" || refusal.Code != "too_long" {
		t.Fatalf("err = %v, want a too_long refusal on blocking_calendars", err)
	}
}

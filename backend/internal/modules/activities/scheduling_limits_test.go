// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
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

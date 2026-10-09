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

// The contract bounds these texts in characters, so accented text that fits
// is accepted and text a character over is refused.
func TestSchedulingTextLimitsCountCharactersNotBytes(t *testing.T) {
	profile := defaultSchedulingProfile()
	profile.Title = strings.Repeat("é", 200)
	profile.Location = strings.Repeat("é", 1000)
	if err := validateSchedulingLimits(profile); err != nil {
		t.Fatalf("profile text at the limit refused: %v", err)
	}
	profile.Title = strings.Repeat("é", 201)
	if err := validateSchedulingLimits(profile); err == nil {
		t.Fatal("a 201-character title was accepted")
	}

	invite := crmcontracts.MeetingInvitationRequest{
		ContactId: openapi_types.UUID(ids.NewV7()), AttendeeEmail: "guest@example.test",
		Subject:     strings.Repeat("é", 200),
		Description: strings.Repeat("é", 5000),
		Location:    strings.Repeat("é", 1000),
	}
	if err := validateInvitation(invite); err != nil {
		t.Fatalf("invitation text at the limit refused: %v", err)
	}
	invite.Subject = strings.Repeat("é", 201)
	if err := validateInvitation(invite); err == nil {
		t.Fatal("a 201-character subject was accepted")
	}
}

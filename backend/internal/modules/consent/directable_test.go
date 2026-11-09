// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestAStaffInstructionCannotWaiveAnUnmetSendBoundary(t *testing.T) {
	for _, role := range []string{"rep", "admin"} {
		t.Run(role, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), principal.Principal{
				Type: principal.PrincipalHuman, ID: "human:reviewer",
				Permissions: principal.Permissions{
					RoleKeys: []string{role}, RowScope: principal.RowScopeAll,
					Objects: map[string]principal.ObjectGrant{entityCommunicationException: {Create: true}},
				},
			})
			if !directableBy(ctx, []RefusedRecipient{{ReasonCode: commsauthz.ReasonNoEvidence}}) {
				t.Fatal("the fixture cannot resolve a missing fact")
			}
			if directableBy(ctx, nil) {
				t.Error("a missing check offers a send instruction")
			}
			for _, reason := range []string{
				commsauthz.ReasonHardBounce, commsauthz.ReasonUnconfirmedDOI,
				commsauthz.ReasonNoSubject, commsauthz.ReasonUnknownPurpose,
				commsauthz.ReasonClaimContradictsResolution,
				commsauthz.ReasonMarketingPermissionRequired,
			} {
				if directableBy(ctx, []RefusedRecipient{{ReasonCode: reason}}) {
					t.Errorf("an instruction waives %s", reason)
				}
			}
		})
	}
}

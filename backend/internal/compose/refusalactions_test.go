// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a refused caller is told they may do about it.
//
// The reference a refusal carries is only useful if the action beside it is one
// this caller can actually take. Both moves are human-only, so an agent told it
// may take one tries, is refused, and was sent there by us.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestARefusalOffersOnlyWhatThisCallerCanDo(t *testing.T) {
	t.Parallel()
	contact := ids.NewV7()
	refused := func(reason string) consent.RefusedRecipient {
		return consent.RefusedRecipient{
			SubjectKind: "contact", SubjectID: contact.String(),
			ReasonCode: reason, Category: string(commsauthz.CategoryRequestedFollowup),
		}
	}
	held := consent.Review{IntentID: ids.NewV7(), Refusals: []consent.RefusedRecipient{refused(commsauthz.ReasonNoEvidence)}}
	mixed := consent.Review{IntentID: held.IntentID, Refusals: []consent.RefusedRecipient{
		refused(commsauthz.ReasonNoEvidence), refused(commsauthz.ReasonHardBounce),
	}}
	bounced := consent.Review{IntentID: held.IntentID, Refusals: []consent.RefusedRecipient{refused(commsauthz.ReasonHardBounce)}}

	for _, tc := range []struct {
		name   string
		actor  principal.Principal
		review consent.Review
		want   []string
	}{
		{
			// A rep overrules a machine refusal with their own reason.
			name: "a human who cannot direct a send", actor: principal.Principal{Type: principal.PrincipalHuman},
			review: held, want: []string{"overrule"},
		},
		{
			// A bounce is a fact about the address that no reason changes.
			name: "a human refused for a reason nobody may overrule", actor: principal.Principal{Type: principal.PrincipalHuman},
			review: bounced,
		},
		{
			// One recipient the rep may not overrule keeps the whole send refused.
			name: "a human refused for one overrulable and one absolute reason", actor: principal.Principal{Type: principal.PrincipalHuman},
			review: mixed,
		},
		{
			// A rep who holds the grant is offered the SEND, not the ask.
			// Telling them to ask a colleague would be telling them to go
			// around themselves.
			name: "a human who may direct a send",
			actor: principal.Principal{
				Type: principal.PrincipalHuman,
				Permissions: principal.Permissions{
					Objects: map[string]principal.ObjectGrant{
						consent.EntityCommunicationException: {Create: true},
					},
				},
			},
			review: held, want: []string{"direct_send"},
		},
		{
			// Holding the grant does not conjure a message to send. A refusal
			// with nothing held offers nothing, whoever is asking.
			name: "a grant holder with no message to decide about",
			actor: principal.Principal{
				Type: principal.PrincipalHuman,
				Permissions: principal.Permissions{
					Objects: map[string]principal.ObjectGrant{
						consent.EntityCommunicationException: {Create: true},
					},
				},
			},
			review: consent.Review{},
		},
		{
			// An agent cannot route: the door refuses non-humans outright.
			name: "an agent", actor: principal.Principal{Type: principal.PrincipalAgent}, review: held,
		},
		{
			// A connector runs with its granting human's grants and is still
			// not a human at a keyboard.
			name: "a connector", actor: principal.Principal{Type: principal.PrincipalConnector}, review: held,
		},
		{
			name: "the system", actor: principal.Principal{Type: principal.PrincipalSystem}, review: held,
		},
		{
			// A refusal with no held message has nothing to decide about — a
			// channel reply, whose shape the held row cannot carry. Offering
			// the route would send even a human into a refusal.
			name:  "a human with no message to decide about",
			actor: principal.Principal{Type: principal.PrincipalHuman}, review: consent.Review{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := principal.WithActor(context.Background(), tc.actor)
			got := actionsForRefusal(ctx, tc.review)
			if len(got) != len(tc.want) {
				t.Fatalf("offered %v, want %v — a caller told it may do something it cannot was "+
					"sent there by us", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("offered %q at %d, want %q", got[i], i, tc.want[i])
				}
			}
		})
	}
}

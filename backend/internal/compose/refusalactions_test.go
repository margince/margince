// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a refused caller is told they may do about it.
//
// The reference a refusal carries is only useful if the action beside it is one
// this caller can take. The one move is the directed send of this one message.
// A caller offered a move it cannot take is refused when it tries.

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
	contact, author := ids.NewV7(), ids.NewV7()
	refused := func(reason string) consent.RefusedRecipient {
		return consent.RefusedRecipient{
			SubjectKind: "contact", SubjectID: contact.String(),
			ReasonCode: reason, Category: string(commsauthz.CategoryRequestedFollowup),
		}
	}
	held := consent.Review{IntentID: ids.NewV7(), InitiatedBy: author, Refusals: []consent.RefusedRecipient{refused(commsauthz.ReasonNoEvidence)}}
	mixed := consent.Review{IntentID: held.IntentID, InitiatedBy: author, Refusals: []consent.RefusedRecipient{
		refused(commsauthz.ReasonNoEvidence), refused(commsauthz.ReasonHardBounce),
	}}
	objected := consent.Review{IntentID: held.IntentID, InitiatedBy: author, Refusals: []consent.RefusedRecipient{refused(commsauthz.ReasonObjection)}}
	objectedBeside := consent.Review{IntentID: held.IntentID, InitiatedBy: author, Refusals: []consent.RefusedRecipient{
		refused(commsauthz.ReasonMarketingConsentRequired), refused(commsauthz.ReasonObjection),
	}}
	holdsTheGrantAs := func(user ids.UUID, roleKeys ...string) principal.Principal {
		return principal.Principal{
			Type: principal.PrincipalHuman, UserID: user,
			Permissions: principal.Permissions{
				RoleKeys: roleKeys,
				Objects: map[string]principal.ObjectGrant{
					consent.EntityCommunicationException: {Create: true},
				},
			},
		}
	}
	holdsTheGrant := func(roleKeys ...string) principal.Principal { return holdsTheGrantAs(author, roleKeys...) }
	colleague := holdsTheGrantAs(ids.NewV7(), "admin")
	bounced := consent.Review{IntentID: held.IntentID, InitiatedBy: author, Refusals: []consent.RefusedRecipient{refused(commsauthz.ReasonHardBounce)}}

	for _, tc := range []struct {
		name   string
		actor  principal.Principal
		review consent.Review
		want   []string
	}{
		{
			// No standing per-contact override is offered in place of the send.
			name: "a human who cannot direct a send", actor: principal.Principal{Type: principal.PrincipalHuman},
			review: held,
		},
		{
			// A bounce is a fact about the address that no reason changes.
			name: "a human refused for a reason nobody may overrule", actor: principal.Principal{Type: principal.PrincipalHuman},
			review: bounced,
		},
		{
			// One recipient no grant directs past keeps the whole send refused.
			name: "a grant holder refused for one directable and one absolute reason", actor: holdsTheGrant("rep"),
			review: mixed,
		},
		{
			// A rep who holds the grant is offered the SEND, not the ask.
			// Telling them to ask a colleague would be telling them to go
			// around themselves.
			name: "a human who may direct a send",
			actor: principal.Principal{
				Type: principal.PrincipalHuman, UserID: author,
				Permissions: principal.Permissions{
					Objects: map[string]principal.ObjectGrant{
						consent.EntityCommunicationException: {Create: true},
					},
				},
			},
			review: held, want: []string{"direct_send"},
		},
		{
			name: "a rep holding the grant, refused for an objection", actor: holdsTheGrant("rep"),
			review: objected, want: []string{"direct_send"},
		},
		{
			name: "an admin, refused for an objection", actor: holdsTheGrant("admin"),
			review: objected, want: []string{"direct_send"},
		},
		{
			name: "an admin, refused for an objection beside a passable refusal", actor: holdsTheGrant("admin"),
			review: objectedBeside, want: []string{"direct_send"},
		},
		// The release sends from the caller's mailbox, so only the author is offered it.
		{name: "an admin holding the grant who did not write the message", actor: colleague, review: held},
		{
			// A bounce is a fact about the address, whoever holds the grant.
			name: "a grant holder refused for a dead address", actor: holdsTheGrant("admin"),
			review: bounced,
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
			// No user behind it, so nobody's send to direct.
			name: "a passportless agent", actor: principal.Principal{Type: principal.PrincipalAgent}, review: held,
		},
		{
			// The decision is the user's own, so their agent is not offered it.
			name: "a user's agent holding the grant",
			actor: principal.Principal{
				Type: principal.PrincipalAgent, PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
				Permissions: principal.Permissions{
					Objects: map[string]principal.ObjectGrant{
						consent.EntityCommunicationException: {Create: true},
					},
				},
			},
			review: held,
		},
		{
			name:   "a user's agent without the grant",
			actor:  principal.Principal{Type: principal.PrincipalAgent, PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7()},
			review: held,
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

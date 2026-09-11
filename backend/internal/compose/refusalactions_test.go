// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a refused caller is told they may do about it.
//
// The reference a refusal carries is only useful if the action beside it is one
// this caller can actually take. Routing a decision is human-only and bound to
// the review's own initiator — so an agent told it may request one tries, is
// refused, and was sent there by us rather than by its own mistake. That is
// worse than being told nothing.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestARefusalOffersOnlyWhatThisCallerCanDo(t *testing.T) {
	t.Parallel()
	held := consent.Review{IntentID: ids.NewV7()}

	for _, tc := range []struct {
		name   string
		actor  principal.Principal
		review consent.Review
		want   []string
	}{
		{
			// A rep with no authority to overrule the engine. Asking somebody
			// who has is the move they can make.
			name: "a person who cannot direct a send", actor: principal.Principal{Type: principal.PrincipalHuman},
			review: held, want: []string{"request_decision"},
		},
		{
			// A rep who holds the grant is offered the SEND, not the ask.
			// Telling them to ask a colleague would be telling them to go
			// around themselves.
			name: "a person who may direct a send",
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
			// not a person at a keyboard.
			name: "a connector", actor: principal.Principal{Type: principal.PrincipalConnector}, review: held,
		},
		{
			name: "the system", actor: principal.Principal{Type: principal.PrincipalSystem}, review: held,
		},
		{
			// A refusal with no held message has nothing to decide about — a
			// channel reply, whose shape the held row cannot carry. Offering
			// the route would send even a person into a refusal.
			name:  "a person with no message to decide about",
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

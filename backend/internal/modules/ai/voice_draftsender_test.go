// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Whose voice a draft is written in.
//
// The rule has one interesting case and it is the one that shipped broken: an
// automation composes under the system principal, which names no person, while
// the mail it drafts still leaves under the automation owner's name. Reading the
// actor alone resolved nothing there, so every automated draft was written in
// nobody's voice — and it failed silently, because a voice that cannot be loaded
// degrades to the plain draft by design.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TestTheVoiceSenderIsTheHumanTheMailGoesOutAs pins the resolution order.
func TestTheVoiceSenderIsTheHumanTheMailGoesOutAs(t *testing.T) {
	rep := ids.NewV7()
	owner := ids.NewV7()

	human := principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + rep.String(), UserID: rep}
	system := principal.Principal{Type: principal.PrincipalSystem, ID: "system"}

	for name, tc := range map[string]struct {
		ctx  context.Context
		want ids.UUID
		ok   bool
	}{
		// The ordinary path: a rep pressing "Write email" is the sender.
		"a human drafting for themselves": {
			ctx:  principal.WithActor(context.Background(), human),
			want: rep,
			ok:   true,
		},
		// The automation path, and the whole reason the sender is bound at all.
		// The actor names no person; the owner does.
		"an automation firing under the system actor": {
			ctx: principal.WithSendingHuman(
				principal.WithActor(context.Background(), system), owner),
			want: owner,
			ok:   true,
		},
		// A human actor CANNOT be overridden. A voice profile holds its owner's
		// verbatim written text, so honouring a bound sender here would turn
		// this value into an authorization input: a call acting as one person
		// would read another person's private writing. The actor already knows
		// whose voice they want.
		"a bound sender never overrides a human actor": {
			ctx: principal.WithSendingHuman(
				principal.WithActor(context.Background(), human), owner),
			want: rep,
			ok:   true,
		},
		// A system job nobody owns has no voice to write in, and guessing one
		// would sign a message in a person who never asked for it.
		"the system actor with no sender bound": {
			ctx:  principal.WithActor(context.Background(), system),
			want: ids.Nil,
			ok:   false,
		},
		"no actor at all": {
			ctx:  context.Background(),
			want: ids.Nil,
			ok:   false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := voiceSender(tc.ctx)
			if ok != tc.ok {
				t.Fatalf("resolved=%t, want %t", ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("the draft would be written in %s's voice, want %s", got, tc.want)
			}
		})
	}
}

// A bound sender cannot make one person's call read another's voice profile.
//
// A profile carries its owner's verbatim written text — the personality they
// typed and excerpts of their own mail — so "whose profile is loaded" is a read
// of somebody's private writing. The sending human exists for a principal that
// names nobody, and letting it override a principal that DOES name somebody
// would make it an authorization input.
func TestABoundSenderCannotRedirectAHumansVoiceRead(t *testing.T) {
	rep := ids.NewV7()
	victim := ids.NewV7()

	ctx := principal.WithSendingHuman(
		principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + rep.String(), UserID: rep,
		}), victim)

	got, ok := voiceSender(ctx)
	if !ok {
		t.Fatal("a human call resolved no sender at all")
	}
	if got == victim {
		t.Fatalf("a bound sender redirected the voice read to %s, whose profile holds their own "+
			"verbatim writing — this value selects whose private text is loaded and must never "+
			"override an actor who names a person", victim)
	}
	if got != rep {
		t.Errorf("the voice read resolved %s, want the acting rep %s", got, rep)
	}
}

// A zero sending human is not a sender.
//
// The automation engine binds the owner only when the firing has one, but a
// caller that bound a zero id anyway must not resolve to it: every voice profile
// row is owned by a real user, so a zero id would read as "no profile" instead
// of falling through to the actor who does have one.
func TestAZeroSendingHumanFallsThroughToTheActor(t *testing.T) {
	rep := ids.NewV7()
	ctx := principal.WithSendingHuman(
		principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + rep.String(), UserID: rep,
		}), ids.Nil)

	got, ok := voiceSender(ctx)
	if !ok || got != rep {
		t.Errorf("a zero sending human resolved to %s/%t, want the acting rep %s", got, ok, rep)
	}
}

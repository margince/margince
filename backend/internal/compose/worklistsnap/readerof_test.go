// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package worklistsnap

// A snapshot binds ONE reader, so readerOf answers a contact and refuses
// everything else. Both halves of that are load-bearing and only one of them
// is auth.RequireHuman's: it refuses a buyer and an agent, and ADMITS the
// system and connector principals, whose zero user id would key the shared row
// this function exists to prevent.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyAContactHoldsAWalk(t *testing.T) {
	me := ids.NewV7()
	for _, tc := range []struct {
		name    string
		actor   principal.Principal
		admit   bool
		because string
	}{
		{
			"a seated human",
			principal.Principal{
				Type: principal.PrincipalHuman, UserID: me,
			},
			true,
			"the walk belongs to them",
		},
		{
			"a human with no user id",
			principal.Principal{
				Type: principal.PrincipalHuman,
			},
			false,
			"there is no id to key the row on",
		},
		{
			"an agent on a passport",
			principal.Principal{
				Type: principal.PrincipalAgent, UserID: me,
			},
			false,
			"a passport carries its human's id and would resume their walk",
		},
		{
			"the system principal carrying a user id",
			principal.Principal{
				Type: principal.PrincipalSystem, ID: "system:sweep", UserID: me,
			},
			false,
			"auth.RequireHuman ADMITS it, so only the explicit type test refuses " +
				"it — and a background pass given somebody's id would resume their walk",
		},
		{
			"a connector carrying a user id",
			principal.Principal{
				Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: me,
			},
			false,
			"the same: RequireHuman admits a connector, and only the type test does not",
		},
		{
			"a Deal Room buyer",
			principal.Principal{
				Type: principal.PrincipalBuyer, UserID: me,
			},
			false,
			"an external participant has no queue here at all",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), tc.actor)
			got, err := readerOf(ctx)
			if tc.admit {
				if err != nil {
					t.Fatalf("readerOf refused %s → %v, want the reader (%s)", tc.name, err, tc.because)
				}
				if got != me {
					t.Errorf("readerOf answered %s, want %s", got, me)
				}
				return
			}
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("readerOf admitted %s → %v, want ErrPermissionDenied: %s",
					tc.name, err, tc.because)
			}
		})
	}
}

// A context with no actor at all is refused rather than panicking, which is
// the shape a misrouted background job arrives in.
func TestAWalkNeedsAnActor(t *testing.T) {
	if _, err := readerOf(context.Background()); err == nil {
		t.Error("readerOf answered a context carrying no principal")
	}
}

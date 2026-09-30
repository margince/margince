// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Who may ask what a legal hold is preserving — answered before any database
// is opened, which is why this is a unit test and not an integration one.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The list names held contacts and leads, so it discloses somebody's existence
// to whoever can read it. An agent is refused at the door for that reason, and
// the refusal has to land BEFORE the read — a nil handle is passed on purpose,
// so a version that reached the database would panic rather than pass.
func TestTheHoldListRefusesAnAgentBeforeItReadsAnything(t *testing.T) {
	t.Parallel()
	for _, k := range []struct {
		why   string
		actor func(context.Context) context.Context
	}{
		{"an agent principal", func(ctx context.Context) context.Context {
			return principal.WithActor(ctx, principal.Principal{
				Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(),
			})
		}},
		{"no principal at all", func(ctx context.Context) context.Context { return ctx }},
	} {
		t.Run(k.why, func(t *testing.T) {
			t.Parallel()
			_, err := ListLegalHolds(k.actor(context.Background()), nil, nil, nil)
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("reading the hold list as %s returned %v, want ErrPermissionDenied", k.why, err)
			}
		})
	}
}

// A human with no retention authority is refused too, and also before the
// read. The two refusals are different questions — WHAT the caller is, and
// WHAT they hold — and a gate answering only the first would let any signed-in
// seat enumerate held records.
func TestTheHoldListRefusesAHumanWithoutTheRetentionAuthority(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(),
	})
	_, err := ListLegalHolds(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("a human holding no grant read the hold list")
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return
	}
	t.Errorf("refused with %v, want ErrPermissionDenied", err)
}

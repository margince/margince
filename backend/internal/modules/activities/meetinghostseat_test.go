// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func hostClaimActor(kind principal.PrincipalType) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: kind, ID: "actor:test"})
}

// No human grant covers a colleague's meeting row, so a human never names its
// host, and the refusal comes before any statement.
func TestAHumanCannotNameAMeetingsHost(t *testing.T) {
	var noTx pgx.Tx
	moved, err := (&Store{}).ClaimOrganizerHostTx(hostClaimActor(principal.PrincipalHuman), noTx, ids.New[ids.ActivityKind]())
	if moved || !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a human's claim answered moved=%v, err=%v, want permission denied", moved, err)
	}
}

// A meeting that is gone has no host to move, which is an answer. A database
// that failed is not: the sync must hear it rather than read "nothing moved".
func TestAFailedLockIsToldApartFromAMeetingThatIsGone(t *testing.T) {
	ctx := hostClaimActor(principal.PrincipalConnector)
	id := ids.New[ids.ActivityKind]()

	moved, err := (&Store{}).ClaimOrganizerHostTx(ctx, unreachableTx{err: pgx.ErrNoRows}, id)
	if moved || err != nil {
		t.Errorf("a meeting that is gone answered moved=%v, err=%v, want neither", moved, err)
	}

	down := errors.New("the database went away")
	moved, err = (&Store{}).ClaimOrganizerHostTx(ctx, unreachableTx{err: down}, id)
	if moved || !errors.Is(err, down) {
		t.Errorf("a failed lock answered moved=%v, err=%v, want the failure", moved, err)
	}
}

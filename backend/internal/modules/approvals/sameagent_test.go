// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// connected builds the staged row and the caller for one connection, with the
// caller's passport supplied separately: rotation is the case where those two
// passports differ and everything else is identical.
func connected(connection ids.UUID, staged, calling ids.UUID) (row, principal.Principal) {
	stagedID := ids.From[ids.PassportKind](staged)
	a := row{
		Kind:         "advance_deal",
		PassportID:   &stagedID,
		ConnectionID: &connection,
	}
	p := principal.Principal{
		Type:         principal.PrincipalAgent,
		PassportID:   calling,
		ConnectionID: connection,
		Scopes:       principal.NewScopeSet(principal.ScopeWrite),
	}
	return a, p
}

// The rule the whole file exists for. A connected agent's passport id is not
// stable: refreshing spends the token and mints a replacement under the same
// grant, so an agent that waits out one access-token lifetime presents a
// different passport with the same authority. Compared on the passport alone,
// that agent releases the confirm-first call it staged itself.
func TestARotatedCredentialDoesNotReleaseWhatItStaged(t *testing.T) {
	connection := ids.NewV7()
	a, rotated := connected(connection, ids.NewV7(), ids.NewV7())

	err := agentMayDecide(rotated, a, true)
	if err == nil {
		t.Fatal("a credential released the proposal it staged, having done nothing but refresh its token")
	}
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("refused with %v, want a permission denial", err)
	}
}

// The mirror, and the reason this is one change rather than a patch to the
// refusal: the same comparison decides whether an agent may REDEEM what a human
// approved for it. Read as passport equality, a rotation told the proposer its
// own authority belonged to somebody else.
func TestARotatedCredentialStillRedeemsWhatItStaged(t *testing.T) {
	connection := ids.NewV7()
	a, rotated := connected(connection, ids.NewV7(), ids.NewV7())
	a.Status = approvalStatusApproved
	a.DiffHash = "h"
	a.ExpiresAt = time.Now().Add(time.Hour)

	if err := validateRedemption(a, rotated, a.Kind, a.DiffHash, time.Now()); err != nil {
		t.Fatalf("an agent was refused the authority a human granted it, because its token had refreshed: %v", err)
	}
}

// Rotation is the only thing sameAgent adds. A passport a human minted directly
// carries no connection and answers to its own id alone, so two of them remain
// two agents — which is what keeps this change out of the open question about
// whether one human's two lent passports may confirm each other.
func TestSameAgentIsConnectionThenPassport(t *testing.T) {
	connection, other := ids.NewV7(), ids.NewV7()
	passport := ids.NewV7()
	passportID := ids.From[ids.PassportKind](passport)

	for name, tc := range map[string]struct {
		a    row
		p    principal.Principal
		want bool
	}{
		"a rotated passport under one connection": {
			a:    row{PassportID: ptr(ids.From[ids.PassportKind](ids.NewV7())), ConnectionID: &connection},
			p:    principal.Principal{PassportID: ids.NewV7(), ConnectionID: connection},
			want: true,
		},
		"a second connection is a second agent": {
			a:    row{PassportID: &passportID, ConnectionID: &connection},
			p:    principal.Principal{PassportID: ids.NewV7(), ConnectionID: other},
			want: false,
		},
		"a directly minted passport answers to its own id": {
			a:    row{PassportID: &passportID},
			p:    principal.Principal{PassportID: passport},
			want: true,
		},
		"two directly minted passports are two agents": {
			a:    row{PassportID: &passportID},
			p:    principal.Principal{PassportID: ids.NewV7()},
			want: false,
		},
		// The zero-value trap: a server-staged row carries neither id, and a
		// human carries neither either. Comparing them as values would make
		// every human the author of every unattended proposal.
		"the server's proposal is nobody's own": {
			a:    row{},
			p:    principal.Principal{Type: principal.PrincipalHuman, UserID: ids.NewV7()},
			want: false,
		},
		"a human deciding a connected agent's proposal": {
			a:    row{PassportID: &passportID, ConnectionID: &connection},
			p:    principal.Principal{Type: principal.PrincipalHuman, UserID: ids.NewV7()},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := sameAgent(tc.a, tc.p); got != tc.want {
				t.Fatalf("sameAgent → %v, want %v", got, tc.want)
			}
		})
	}
}

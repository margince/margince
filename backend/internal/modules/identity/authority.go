// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The shared/ports/authz implementation: identity owns the role /
// role_assignment / app_user.seat_type tables, so it answers the gate's
// live authority questions (interfaces.md §2). platform/auth consumes
// only the interface; the composition root wires this Service in — the
// DAG never gains a platform→modules edge.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

var _ authz.Resolver = (*Service)(nil)

// RBACObjectGrantable reports whether an object name is one a role document may
// grant at all. Identity owns that vocabulary (internal/policy's coreObjects,
// which Parse rejects a document for stepping outside), so it is the only module
// that can answer.
//
// It is exported for the gates that derive an authority requirement somewhere
// else and must prove the requirement is SATISFIABLE before certifying whatever
// depends on it. An object outside this set is allowed by no principal that can
// exist, so a requirement naming one is not a strict rule — it is a permanent
// refusal, and a gate reading only the requirement's presence cannot tell the two
// apart. The confirm-first decidability gate is the caller today: an approval
// whose decision demands an ungrantable object can never be released or rejected
// by anyone.
func RBACObjectGrantable(object string) bool {
	return policy.IsGrantableObject(object)
}

// EffectiveRBAC reads the human's CURRENT role grants + teams. A user who
// is archived, suspended, or outside the context workspace resolves to
// ErrNotFound — absence of authority is denial, not empty permission.
func (s *Service) EffectiveRBAC(ctx context.Context, workspaceID, humanID ids.UUID) (authz.RBAC, error) {
	var out authz.RBAC
	err := s.liveUserTx(ctx, workspaceID, humanID, func(tx pgx.Tx, _ string) error {
		_, teams, perms, err := loadGrants(ctx, tx, ids.From[ids.UserKind](humanID))
		if err != nil {
			return err
		}
		out = authz.RBAC{Permissions: perms, TeamIDs: rawTeamIDs(teams)}
		return nil
	})
	return out, err
}

// EffectiveAuthority reads the human's grants AND their seat in ONE snapshot.
//
// The pair exists because reading them separately can compose an authority the
// member never held: a role change and a seat change that cross between two
// transactions leave a caller holding permissions from before with a seat from
// after. Both are ceilings on the same act, so they have to be read together —
// and liveUserTx already has both in hand, which is what makes this the cheap
// answer rather than a new query.
//
// A caller that legitimately needs only one may still use EffectiveRBAC or
// SeatType; what must not happen is one caller reading both, separately, and
// treating the pair as a fact about one instant.
func (s *Service) EffectiveAuthority(ctx context.Context, workspaceID, humanID ids.UUID) (authz.RBAC, principal.SeatType, error) {
	var (
		rbac authz.RBAC
		seat principal.SeatType
	)
	err := s.liveUserTx(ctx, workspaceID, humanID, func(tx pgx.Tx, seatType string) error {
		_, teams, perms, err := loadGrants(ctx, tx, ids.From[ids.UserKind](humanID))
		if err != nil {
			return err
		}
		rbac = authz.RBAC{Permissions: perms, TeamIDs: rawTeamIDs(teams)}
		seat = principal.SeatType(seatType)
		return nil
	})
	return rbac, seat, err
}

// AdmittedAuthority answers the whole admission question in one snapshot: the
// passport is still live, and this is the granting human's seat and RBAC as of
// that same instant.
//
// The passport is re-asked HERE, inside the transaction the seat read already
// opens — one more statement on a connection already held, rather than a third
// round trip through the pool. That matters because it runs on every tool call:
// a run authenticates once at start and then executes for its whole wall clock,
// so this asking is the "next token lookup" revocation is documented as binding
// at. Against what it replaces it is not a new cost at all: the gate used to
// make TWO transactions here, for the seat and the grants, and now makes one.
//
// It reuses passportStillLiveQuery rather than restating the rule, so a
// condition added to authentication reaches admission without anybody
// remembering to come here.
func (s *Service) AdmittedAuthority(ctx context.Context, workspaceID, humanID, passportID ids.UUID) (authz.RBAC, principal.SeatType, error) {
	var (
		rbac authz.RBAC
		seat principal.SeatType
	)
	err := s.liveUserTx(ctx, workspaceID, humanID, func(tx pgx.Tx, seatType string) error {
		// A ZERO passport is a principal holding no credential — the product
		// acting under a policy, derived from a live human at construction
		// rather than from a token somebody can revoke. There is nothing to
		// re-ask about, and asking anyway would refuse every such call.
		// platform/auth's Admit names the two paths that mint one.
		if !passportID.IsZero() {
			var live int
			err := tx.QueryRow(ctx, passportStillLiveQuery, passportID, humanID).Scan(&live)
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		_, teams, perms, err := loadGrants(ctx, tx, ids.From[ids.UserKind](humanID))
		if err != nil {
			return err
		}
		rbac = authz.RBAC{Permissions: perms, TeamIDs: rawTeamIDs(teams)}
		seat = principal.SeatType(seatType)
		return nil
	})
	return rbac, seat, err
}

// SeatType reads the human's current seat — the A62/ADR-0047 licensing
// ceiling the gate checks before any tier reasoning.
func (s *Service) SeatType(ctx context.Context, workspaceID, humanID ids.UUID) (principal.SeatType, error) {
	var seat principal.SeatType
	err := s.liveUserTx(ctx, workspaceID, humanID, func(_ pgx.Tx, seatType string) error {
		seat = principal.SeatType(seatType)
		return nil
	})
	return seat, err
}

// liveUserTx runs fn inside the workspace transaction after proving the
// user is live in exactly the requested workspace. The GUC contract binds
// the tenant from the context; a caller asking about a different
// workspace is a programming error, refused rather than answered from
// the wrong tenant.
func (s *Service) liveUserTx(ctx context.Context, workspaceID, humanID ids.UUID, fn func(tx pgx.Tx, seatType string) error) error {
	ctxWs, ok := principal.WorkspaceID(ctx)
	if !ok || ctxWs != workspaceID {
		return fmt.Errorf("crmauth: authority resolution outside the bound workspace")
	}
	// ONE SNAPSHOT, and it has to be asked for. The pool begins at READ
	// COMMITTED, where every statement sees its own committed view — so a seat
	// read, a passport read and a grant read in one transaction can return
	// values that never existed together: permissions from before a role change
	// beside a seat from after it, or a live passport beside grants the human no
	// longer holds.
	//
	// That is not an abstract race. These reads ARE the admission decision, and
	// the composed answer is what the caller is admitted on; EffectiveAuthority's
	// own comment says the pair must be read together, and this is what makes
	// that true rather than intended.
	//
	// At BEGIN, through TxIsolated, rather than as a statement at the top of
	// this closure. Postgres refuses the level once any query has taken a
	// snapshot, and Tx runs one of its own on a BOUNDED handle — so a pin
	// spelled here would work on the handle identity has today and fail on the
	// one compose could give it tomorrow. Read-only work at REPEATABLE READ
	// takes its snapshot at the first statement and cannot serialize-fail.
	return s.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		var seatType string
		err := tx.QueryRow(ctx,
			`SELECT seat_type FROM app_user
			 WHERE id = $1 AND `+LiveMemberSQL("")+``,
			humanID).Scan(&seatType)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		return fn(tx, seatType)
	})
}

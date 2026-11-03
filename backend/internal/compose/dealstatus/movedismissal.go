// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The deal card's "Not now": one reader setting the card's move aside.
//
// The contact and company cards follow the same rule. It is per reader: a
// rep's own day must not change a colleague's card. It lifts by itself when
// the card names a different move. It is view state, so like the other two
// stores it writes no audit row and emits no event.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DismissDealMove implements POST /deals/{id}/move/dismiss.
func (h Handlers) DismissDealMove(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.DismissDealMoveRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.svc.DismissMove(r.Context(), ids.From[ids.DealKind](ids.UUID(id)), body.Fingerprint); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreDealMove implements DELETE /deals/{id}/move/dismiss.
func (h Handlers) RestoreDealMove(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.RestoreDealMoveParams) {
	if err := h.svc.RestoreMove(r.Context(), ids.From[ids.DealKind](ids.UUID(id)), params.Fingerprint); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DismissMove records that this reader has set the deal's move aside.
//
// The fingerprint is not checked against the card the deal would draw now:
// that would mean assembling the card, model call included, for a click. The
// table stays bounded anyway, at one row per reader and deal, and a digest
// that matches no move hides nothing.
func (s *Service) DismissMove(ctx context.Context, dealID ids.DealID, fingerprint string) error {
	return s.setAside(ctx, dealID, fingerprint, `
		INSERT INTO deal_move_dismissal (user_id, deal_id, fingerprint)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, deal_id)
		DO UPDATE SET fingerprint = EXCLUDED.fingerprint, dismissed_at = now()`)
}

// RestoreMove clears this reader's set-aside of the move with this digest:
// the undo behind "Not now". A newer set-aside of a different move stays.
func (s *Service) RestoreMove(ctx context.Context, dealID ids.DealID, fingerprint string) error {
	return s.setAside(ctx, dealID, fingerprint, `
		DELETE FROM deal_move_dismissal
		WHERE user_id = $1 AND deal_id = $2 AND fingerprint = $3`)
}

// setAside runs one write of the set-aside behind the gates both verbs share.
// So the readers who may set a move aside are the ones who may undo it.
func (s *Service) setAside(ctx context.Context, dealID ids.DealID, fingerprint, statement string) error {
	// A passport carries its granting human's id, so an agent's set-aside
	// would hide a move on a human's page that the human never saw.
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Setting aside a move on a deal the caller cannot read would confirm
		// the deal exists, so the row gate comes first.
		if err := auth.EnsureVisible(ctx, tx, "deal", dealID.UUID); err != nil {
			return err
		}
		if !moveFingerprintPattern.MatchString(fingerprint) {
			return httperr.Validation("fingerprint", "malformed",
				"name the move by the fingerprint the card served it with, unchanged")
		}
		if _, err := tx.Exec(ctx, statement, userID, dealID, fingerprint); err != nil {
			return fmt.Errorf("write the deal move set-aside: %w", err)
		}
		return nil
	})
}

// moveFingerprintPattern is the shape moveFingerprint produces: a sha256
// digest in lowercase hex. A malformed value is a stated 422 rather than a
// row the check constraint refuses with a 500.
var moveFingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// withSetAside drops the card's move when this reader set that very move
// aside. It asks MovesSetAside, the question the Worklist's deal rows ask.
func (s *Service) withSetAside(
	ctx context.Context, dealID ids.DealID, card crmcontracts.DealStatusCard,
) (crmcontracts.DealStatusCard, error) {
	if card.Next == nil || card.Next.Action == ActionNone {
		return card, nil
	}
	next := *card.Next
	if next.Fingerprint == nil {
		// A card cached before moves were stamped is digested as it is served.
		digest := moveFingerprint(next)
		next.Fingerprint = &digest
	}
	card.Next = &next
	var setAside map[ids.UUID]bool
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		setAside, err = MovesSetAside(ctx, tx, map[ids.UUID]crmcontracts.DealStatusCardMove{dealID.UUID: next})
		return err
	})
	if err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	if setAside[dealID.UUID] {
		card.Next = nil
	}
	return card, nil
}

// MovesSetAside reports which of these deals' moves the reader this context
// authenticates has set aside, keyed by deal id. A move counts only while its
// fingerprint matches the one stored, so a different move on the deal shows.
//
// Only a human's own set-aside applies, as on the contact page. An agent
// reading a deal sees every move, including one a human put away, so for any
// other principal the answer is empty.
func MovesSetAside(
	ctx context.Context, tx pgx.Tx, moves map[ids.UUID]crmcontracts.DealStatusCardMove,
) (map[ids.UUID]bool, error) {
	out := map[ids.UUID]bool{}
	if len(moves) == 0 {
		return out, nil
	}
	if err := auth.RequireHuman(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return nil, err
	}
	dealIDs := make([]ids.UUID, 0, len(moves))
	for dealID := range moves {
		dealIDs = append(dealIDs, dealID)
	}
	rows, err := tx.Query(ctx, `
		SELECT deal_id, fingerprint FROM deal_move_dismissal
		WHERE user_id = $1 AND deal_id = ANY($2)`,
		userID, dealIDs)
	if err != nil {
		return nil, fmt.Errorf("read the deal move set-asides: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dealID ids.UUID
		var stored string
		if err := rows.Scan(&dealID, &stored); err != nil {
			return nil, fmt.Errorf("read the deal move set-asides: %w", err)
		}
		out[dealID] = stored == fingerprintOf(moves[dealID])
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the deal move set-asides: %w", err)
	}
	return out, nil
}

// fingerprintOf is the digest a move is set aside by. It is the one stamped
// when the rules chose it. A card cached before stamping is digested now.
func fingerprintOf(move crmcontracts.DealStatusCardMove) string {
	if move.Fingerprint != nil {
		return *move.Fingerprint
	}
	return moveFingerprint(move)
}

// moveFingerprint digests what the move names: its verb, its operands and the
// records it rests on. The rule that chose it may add marks of its own. The
// reason is left out: it counts days, and a model may rewrite it.
func moveFingerprint(move crmcontracts.DealStatusCardMove, extra ...owedwork.Mark) string {
	operands := "{}"
	if move.Arguments != nil {
		// %v prints map keys in sorted order, so the same operands always
		// spell the same, whether built here or decoded from the cache.
		operands = fmt.Sprintf("%v", semanticArguments(*move.Arguments))
	}
	marks := []owedwork.Mark{{Kind: move.Action, ID: operands}}
	for _, evidence := range move.Evidence {
		mark := owedwork.Mark{Kind: "evidence", ID: evidence.Text}
		if evidence.ActivityId != nil {
			mark.ID = evidence.ActivityId.String()
		}
		marks = append(marks, mark)
	}
	return owedwork.Fingerprint(append(marks, extra...))
}

// stamped is the move with the digest a reader sets it aside by. It is taken
// from the rules' own move, before a model may rewrite its reason or citations.
// So the digest names what clicking does, not how it was explained.
func stamped(move crmcontracts.DealStatusCardMove, extra ...owedwork.Mark) crmcontracts.DealStatusCardMove {
	if move.Action == ActionNone {
		return move
	}
	digest := moveFingerprint(move, extra...)
	move.Fingerprint = &digest
	return move
}

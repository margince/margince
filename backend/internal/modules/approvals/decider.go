// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ErrNoDecider refuses a staging that names nobody responsible for it. A system
// job meets it on a record nobody owns, and then stages nothing.
var ErrNoDecider = errors.New("approvals: nobody is responsible for this request; it names no member and its record has no owner")

// deciderSource is which rule named the member, stored beside them because
// only a record owner follows the record when its owner changes.
type deciderSource string

const (
	deciderPayloadOwner deciderSource = "payload_owner"
	deciderOnBehalfOf   deciderSource = "on_behalf_of"
	deciderStager       deciderSource = "stager"
	deciderRecordOwner  deciderSource = "record_owner"
)

type deciderCandidate struct {
	member ids.UUID
	source deciderSource
}

// resolveDecider names the one member a staging is for. A payload-owned kind
// has one candidate, its payload's owner, because Withheld lets nobody else
// open it. Any other kind tries, in order: the human the work is done for, the
// human staging it, the owner of the record it changes. The first who is or may
// become active wins, so an invited colleague finds the request waiting.
func resolveDecider(ctx context.Context, tx pgx.Tx, p principal.Principal, in StageInput) (deciderCandidate, error) {
	if approvalsubject.PayloadOwned(in.Kind) {
		return firstAskable(ctx, tx, []deciderCandidate{{approvalsubject.Owner(in.Kind, in.ProposedChange), deciderPayloadOwner}})
	}
	candidates := []deciderCandidate{{p.OnBehalfOf, deciderOnBehalfOf}}
	if p.Type == principal.PrincipalHuman {
		candidates = append(candidates, deciderCandidate{p.UserID, deciderStager})
	}
	owner, err := targetOwner(ctx, tx, in.TargetType, in.TargetID)
	if err != nil {
		return deciderCandidate{}, err
	}
	candidates = append(candidates, deciderCandidate{owner, deciderRecordOwner})
	return firstAskable(ctx, tx, candidates)
}

func firstAskable(ctx context.Context, tx pgx.Tx, candidates []deciderCandidate) (deciderCandidate, error) {
	for _, c := range candidates {
		askable, err := CanBeAsked(ctx, tx, c.member)
		if err != nil {
			return deciderCandidate{}, err
		}
		if askable {
			return c, nil
		}
	}
	return deciderCandidate{}, ErrNoDecider
}

// targetOwner reads owner_id from a target whose own row carries one, the same
// classification targetVisible probes by; any other target has no owner. The
// share lock makes a concurrent owner change either land first or wait, so a
// request never names an owner its record has already left.
func targetOwner(ctx context.Context, tx pgx.Tx, targetType string, targetID ids.UUID) (ids.UUID, error) {
	if targetID.IsZero() || probeFor(targetType) != probeOwnScope {
		return ids.Nil, nil
	}
	var owner *ids.UUID
	err := tx.QueryRow(ctx, `SELECT owner_id FROM `+pgx.Identifier{targetType}.Sanitize()+` WHERE id = $1 FOR SHARE`,
		targetID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil || owner == nil {
		return ids.Nil, err
	}
	return *owner, nil
}

// CanBeAsked reports whether a request may name this member
// (workforce.AskableMemberSQL).
func CanBeAsked(ctx context.Context, tx pgx.Tx, colleague ids.UUID) (bool, error) {
	if colleague.IsZero() {
		return false, nil
	}
	var activatable bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_user
		 WHERE id = $1 AND `+workforce.AskableMemberSQL("")+`)`, colleague).Scan(&activatable)
	return activatable, err
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// CarryOwnerAccessTx keeps a former owner's access to a record a merge folded
// theirs into, at the widest access their seat allows. A grant they already
// hold keeps its term, reason and grantor; only its access may rise. A new grant
// is persistent. A colleague who has left gets nothing. mergedFrom may be zero
// when no single record retired.
func (s *Service) CarryOwnerAccessTx(ctx context.Context, tx pgx.Tx, recordType string, record, owner, mergedFrom ids.UUID) (authz.OwnerAccessCarry, error) {
	out := authz.OwnerAccessCarry{Owner: owner}
	if !shareableRecordTypes[recordType] || record.IsZero() || owner.IsZero() {
		return out, fmt.Errorf("identity: an owner carry needs a shareable record and an owner")
	}
	// The two questions a share asks of its grantor. Any principal may pass:
	// keeping an owner's own access is part of a merge it may already make.
	if err := auth.EnsureLinkTarget(ctx, tx, recordType, record); err != nil {
		return out, err
	}
	if err := auth.EnsureCanGrant(ctx, tx, recordType, record); err != nil {
		return out, err
	}
	live, err := memberLive(ctx, tx, owner)
	if err != nil {
		return out, err
	}
	if !live {
		out.Skipped = authz.OwnerCarryInactive
		return out, nil
	}
	access, err := ShareAccessForTx(ctx, tx, owner)
	if err != nil {
		return out, err
	}
	in := CreateGrantInput{RecordType: recordType, RecordID: record, SubjectType: "user", SubjectID: owner, Access: access}
	if err := storekit.LockWriteIdentity(ctx, tx, "record_grant", grantIdentity(in)); err != nil {
		return out, err
	}
	prior, held, err := replacedGrant(ctx, tx, in)
	if err != nil {
		return out, err
	}
	grantedBy := ownerCarryGrantor(ctx, owner)
	var before map[string]any
	if held {
		live, err := grantLiveNow(ctx, tx, prior)
		if err != nil {
			return out, err
		}
		if live {
			if prior.Access == access || access != grantAccessWrite {
				out.Skipped = authz.OwnerCarryAlreadyGranted
				return out, nil
			}
			in.ExpiresAt, in.Reason, grantedBy = prior.ExpiresAt, prior.Reason, prior.GrantedBy
		}
		before = grantImage(prior)
	}
	row, err := upsertGrant(ctx, tx, in, grantedBy)
	if err != nil {
		return out, err
	}
	evidence := map[string]any{"owner_carry": true}
	if !mergedFrom.IsZero() {
		evidence["merged_from"] = mergedFrom
	}
	if _, err := storekit.AuditWithEvidence(ctx, tx, "record_share", recordType, record,
		before, grantImage(row), evidence); err != nil {
		return out, fmt.Errorf("audit the owner access a merge kept: %w", err)
	}
	out.Access = row.Access
	return out, nil
}

// grantAccessWrite is the wider of a grant's two access levels.
const grantAccessWrite = string(crmcontracts.RecordGrantAccessRecordGrantAccessWrite)

// grantLiveNow judges expiry against the database clock, as the visibility
// predicates do.
func grantLiveNow(ctx context.Context, tx pgx.Tx, g grantRow) (bool, error) {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return false, fmt.Errorf("read the database clock: %w", err)
	}
	return grantLive(g, now), nil
}

// ownerCarryGrantor is the human accountable for a carried grant. That is the
// human the merge acts for, or the grantee for a system merge.
func ownerCarryGrantor(ctx context.Context, owner ids.UUID) ids.UUID {
	if actor, ok := principal.Actor(ctx); ok && !actor.UserID.IsZero() {
		return actor.UserID
	}
	return owner
}

func memberLive(ctx context.Context, tx pgx.Tx, member ids.UUID) (bool, error) {
	var live bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_user u WHERE u.id = @member AND u.removed_at IS NULL AND `+
		workforce.LiveMemberSQL("u")+`)`, pgx.NamedArgs{"member": member}).Scan(&live); err != nil {
		return false, fmt.Errorf("read whether the former owner is still a colleague: %w", err)
	}
	return live, nil
}

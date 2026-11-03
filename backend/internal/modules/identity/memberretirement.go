// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MemberWorkInspection separates all dependencies from work the actor may inspect.
type MemberWorkInspection struct {
	Owned        crmcontracts.HandoverCounts
	VisibleOwned crmcontracts.HandoverCounts
	Signals      int
	Blockers     []crmcontracts.MemberRetirementBlocker
	Stamp        string
}

// MemberWork delegates work movement to the modules that own those records.
type MemberWork interface {
	Inspect(context.Context, pgx.Tx, ids.UUID, bool) (MemberWorkInspection, error)
	Retire(context.Context, pgx.Tx, ids.UUID, *ids.UUID, bool) error
}

// WithMemberWork connects retirement to the owning modules without sibling imports.
func (s *Service) WithMemberWork(work MemberWork) *Service { s.memberWork = work; return s }

type retirementMember struct {
	Name    string
	Email   string
	Status  string
	Updated time.Time
	Removed *time.Time
	Merged  *ids.UUID
	Agent   bool
	Unused  bool
}

func readRetirementMember(ctx context.Context, tx pgx.Tx, id ids.UserID, lock bool) (retirementMember, error) {
	var m retirementMember
	query := `SELECT display_name, email, status, updated_at, removed_at, merged_into_user_id, is_agent, ` + unusedMemberSQL("app_user") + ` FROM app_user WHERE id = @member AND archived_at IS NULL`
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, pgx.StrictNamedArgs{memberParameter: id}).Scan(&m.Name, &m.Email, &m.Status, &m.Updated, &m.Removed, &m.Merged, &m.Agent, &m.Unused)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, apperrors.ErrNotFound
	}
	if err != nil {
		return m, err
	}
	if m.Agent {
		return m, errAgentSeatHoldsNoRole
	}
	return m, nil
}

// PreviewMemberRetirement reads a dependency snapshot without taking mutation locks.
func (s *Service) PreviewMemberRetirement(ctx context.Context, actor Identity, source ids.UserID, target *ids.UUID, merge bool) (crmcontracts.MemberRetirementPreview, error) {
	var out crmcontracts.MemberRetirementPreview
	ctx, err := memberRetirementContext(ctx, actor)
	if err != nil {
		return out, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := confirmMemberAdministrator(ctx, tx, actor.UserID); err != nil {
			return err
		}
		var previewErr error
		out, previewErr = s.retirementPreviewTx(ctx, tx, actor, source, target, retirementPreviewOptions{merge: merge})
		return previewErr
	})
	return out, err
}

type retirementPreviewOptions struct{ merge, lock bool }

func (s *Service) retirementPreviewTx(ctx context.Context, tx pgx.Tx, actor Identity, source ids.UserID, target *ids.UUID, options retirementPreviewOptions) (crmcontracts.MemberRetirementPreview, error) {
	merge, lock := options.merge, options.lock
	out := crmcontracts.MemberRetirementPreview{Blockers: []crmcontracts.MemberRetirementBlocker{}, Consequences: []crmcontracts.MemberRetirementConsequence{"source_signed_out", "original_attribution"}}
	if merge {
		out.Consequences = append(out.Consequences, "connectors_reconnect", "survivor_access")
	} else {
		out.Consequences = append(out.Consequences, "private_alerts_archived", "crm_private_records_move", "private_audiences_retained", "email_reserved")
	}
	member, err := readRetirementSource(ctx, tx, actor, source, lock)
	if err != nil {
		return out, err
	}
	out.SourceName = member.Name
	out.SourceEmail = openapi_types.Email(member.Email)
	var survivor retirementMember
	if target != nil {
		survivor, err = readRetirementTarget(ctx, tx, source, *target, lock)
		if err != nil {
			return out, err
		}
		if survivor.Status == userStatusDeactivated {
			out.Consequences = append(out.Consequences, "survivor_inactive")
		}
		out.SurvivorName = &survivor.Name
		email := openapi_types.Email(survivor.Email)
		out.SurvivorEmail = &email
	}
	if s.memberWork == nil {
		return out, apperrors.ErrConflict
	}
	work, err := s.memberWork.Inspect(ctx, tx, source.UUID, lock)
	if err != nil {
		return out, err
	}
	applyRetirementWork(&out, work, member, actor, target, merge)
	workStamp := work.Stamp
	encoded, err := json.Marshal(struct {
		Actor, Source    ids.UUID
		Target           *ids.UUID
		Member, Survivor retirementMember
		Preview          crmcontracts.MemberRetirementPreview
		WorkStamp        string
		Merge            bool
	}{actor.UserID.UUID, source.UUID, target, member, survivor, out, workStamp, merge})
	if err != nil {
		return out, err
	}
	digest := sha256.Sum256(encoded)
	out.Token = hex.EncodeToString(digest[:])
	return out, nil
}

func applyRetirementWork(out *crmcontracts.MemberRetirementPreview, work MemberWorkInspection, member retirementMember, actor Identity, target *ids.UUID, merge bool) {
	out.Owned, out.Blockers = work.VisibleOwned, work.Blockers
	if merge {
		out.Owned.Task = work.Owned.Task
	} else if work.Owned.Task > work.VisibleOwned.Task {
		out.Blockers = append(out.Blockers, "private_tasks")
	}
	if work.Owned.Project > work.VisibleOwned.Project {
		out.Blockers = append(out.Blockers, "work_not_transferable")
	}
	if merge || member.Status == userStatusDeactivated {
		out.SignalCount = work.Signals
	}
	if merge {
		applyMergeEligibility(out, member, actor, target)
	}
	if !merge && member.Status != userStatusDeactivated {
		out.Blockers = append(out.Blockers, "deactivate_first")
	}
	if merge && target == nil {
		out.Blockers = append(out.Blockers, "choose_survivor")
	}
	if target == nil && memberHasWork(work.Owned) {
		out.Blockers = append(out.Blockers, "choose_receiver")
	}
}

func applyMergeEligibility(out *crmcontracts.MemberRetirementPreview, member retirementMember, actor Identity, target *ids.UUID) {
	if !member.Unused {
		out.Blockers = append(out.Blockers, "source_used")
	}
	if target != nil && *target == actor.UserID.UUID {
		out.Blockers = append(out.Blockers, "self_survivor")
	}
}

func memberHasWork(c crmcontracts.HandoverCounts) bool {
	return c.Contact+c.Company+c.Deal+c.Lead+c.Project+c.Task > 0
}

// RetireMember moves work and retires the source as one atomic change.
func (s *Service) RetireMember(ctx context.Context, actor Identity, source ids.UserID, target *ids.UUID, token string, merge bool) error {
	ctx, err := memberRetirementContext(ctx, actor)
	if err != nil {
		return err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error { return s.retireMemberTx(ctx, tx, actor, source, target, token, merge) })
	if err == nil && s.captureReaper != nil {
		s.captureReaper(ctx, source)
	}
	return err
}

func checkRetirementReplay(ctx context.Context, tx pgx.Tx, source ids.UserID, target *ids.UUID, merge bool, before retirementMember) error {
	if merge {
		if target != nil && before.Merged != nil && *target == *before.Merged {
			return nil
		}
		return apperrors.ErrConflict
	}
	if before.Merged != nil {
		return apperrors.ErrConflict
	}
	var recipient *string
	err := tx.QueryRow(ctx, `SELECT after ->> 'work_given_to' FROM audit_log
 WHERE entity_type = 'user' AND entity_id = @source AND action = 'archive' AND after ->> 'removed' = 'true'
 ORDER BY occurred_at DESC, id DESC LIMIT 1`, pgx.StrictNamedArgs{"source": source}).Scan(&recipient)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrConflict
	}
	if err != nil {
		return err
	}
	if target == nil && recipient == nil {
		return nil
	}
	if target != nil && recipient != nil && target.String() == *recipient {
		return nil
	}
	return apperrors.ErrConflict
}

func readRetirementSource(ctx context.Context, tx pgx.Tx, actor Identity, source ids.UserID, lock bool) (retirementMember, error) {
	if source == actor.UserID {
		return retirementMember{}, fmt.Errorf("%w: you cannot remove your own account", apperrors.ErrConflict)
	}
	member, err := readRetirementMember(ctx, tx, source, lock)
	if err != nil {
		return retirementMember{}, err
	}
	if member.Removed != nil {
		return retirementMember{}, apperrors.ErrNotFound
	}
	last, err := soleActiveAdmin(ctx, tx, source)
	if lock {
		last, err = lastActiveAdmin(ctx, tx, source)
	}
	if err != nil {
		return retirementMember{}, err
	}
	if last {
		return retirementMember{}, errLastActiveAdmin
	}
	return member, nil
}

func readRetirementTarget(ctx context.Context, tx pgx.Tx, source ids.UserID, target ids.UUID, lock bool) (retirementMember, error) {
	if target == source.UUID {
		return retirementMember{}, fmt.Errorf("%w: choose a different surviving member", apperrors.ErrConflict)
	}
	survivor, err := readRetirementMember(ctx, tx, ids.From[ids.UserKind](target), lock)
	if err != nil {
		return retirementMember{}, err
	}
	if survivor.Removed != nil {
		return retirementMember{}, apperrors.ErrNotFound
	}
	return survivor, nil
}

func (s *Service) retireMemberTx(ctx context.Context, tx pgx.Tx, actor Identity, source ids.UserID, target *ids.UUID, token string, merge bool) error {
	if err := lockAuthorization(ctx, tx); err != nil {
		return err
	}
	if err := confirmMemberAdministrator(ctx, tx, actor.UserID); err != nil {
		return err
	}
	before, err := readRetirementMember(ctx, tx, source, true)
	if err != nil {
		return err
	}
	if before.Removed != nil {
		return checkRetirementReplay(ctx, tx, source, target, merge, before)
	}
	preview, err := s.retirementPreviewTx(ctx, tx, actor, source, target, retirementPreviewOptions{merge: merge, lock: true})
	if err != nil {
		return err
	}
	if token == "" || token != preview.Token {
		return apperrors.ErrVersionSkew
	}
	if len(preview.Blockers) > 0 {
		return fmt.Errorf("%w: resolve the items in the removal preview first", apperrors.ErrConflict)
	}
	if merge && target == nil {
		return apperrors.ErrConflict
	}
	return s.applyMemberRetirement(ctx, tx, actor, source, target, preview, before, merge)
}

func (s *Service) applyMemberRetirement(ctx context.Context, tx pgx.Tx, actor Identity, source ids.UserID, target *ids.UUID, preview crmcontracts.MemberRetirementPreview, before retirementMember, merge bool) error {
	if _, err := tx.Exec(ctx, `UPDATE app_user SET status = 'deactivated' WHERE id = @member`, pgx.StrictNamedArgs{memberParameter: source}); err != nil {
		return err
	}
	if err := s.memberWork.Retire(ctx, tx, source.UUID, target, merge); err != nil {
		return err
	}
	if err := s.revokeBorrowedAuthority(ctx, tx, source); err != nil {
		return err
	}
	var merged *ids.UUID
	if merge {
		merged = target
	}
	if _, err := tx.Exec(ctx, `UPDATE app_user SET removed_at = now(), merged_into_user_id = @merged WHERE id = @member`, pgx.StrictNamedArgs{memberParameter: source, "merged": merged}); err != nil {
		return err
	}
	auditID, err := storekit.Audit(ctx, tx, "archive", "user", source.UUID,
		map[string]string{"display_name": preview.SourceName, "status": before.Status}, map[string]any{"removed": true, "merged_into_user_id": merged, "work_given_to": target})
	if err != nil {
		return err
	}
	if merge && target != nil {
		if err := recordMemberSurvivor(ctx, tx, actor, source, *target); err != nil {
			return err
		}
	}

	reason := "account removed"
	if merge {
		reason = "duplicate account merged"
	}
	return storekit.EmitEvent(ctx, tx, auditID, source.UUID, userDeactivatedPayload(source, actor.UserID, &reason))
}

func recordMemberSurvivor(ctx context.Context, tx pgx.Tx, actor Identity, source ids.UserID, target ids.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE app_user SET updated_at = now() WHERE id = @id`, pgx.StrictNamedArgs{"id": target}); err != nil {
		return err
	}
	survivorAudit, err := storekit.Audit(ctx, tx, "merge", "user", target, nil, map[string]ids.UUID{mergedFromField: source.UUID})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, survivorAudit, target, crmcontracts.PublicEventUserUpdated{UserId: openapi_types.UUID(target), By: openapi_types.UUID(actor.UserID.UUID)})
}

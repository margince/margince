// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal archive, split out of deal.go when that file reached the
// file-length cap.
//
// The refusal and the write live together because they are one obligation read
// twice: what the store refuses at execution is what a confirm-first staging
// has to be able to ask BEFORE a human answers, and the two drifting apart is
// the defect the pair exists to prevent.

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
)

const (
	dealArchiveRelationships = "deal_id = @record AND archived_at IS NULL"
	dealArchiveMemberships   = "entity_type = 'deal' AND entity_id = @record"
)

// RefuseArchiveDeal answers every authority refusal ArchiveDeal would answer
// with, and writes nothing — the stage-time half of the archive, so a staged
// approval is never spent on a call the store was always going to refuse. No
// version probe: see RefuseArchiveActivity on why a pin is the write's
// business rather than a staging's.
func (s *Store) RefuseArchiveDeal(ctx context.Context, id ids.DealID) error {
	if err := auth.Require(ctx, "deal", principal.ActionDelete); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureWritableLive(ctx, tx, dealTable, id.UUID)
	})
}

// ArchiveDeal retires one deal and the edges hanging off it, conditioned on
// ifVersion wherever the caller's authority named a version.
//
// The DEAL's own row rides the guarded patch; the relationship sweep below
// stays a plain statement because it is a cascade off that row rather than a
// second decision — the guard on the deal is what serializes both.
func (s *Store) ArchiveDeal(ctx context.Context, id ids.DealID, ifVersion *int64, opts ...ArchiveOption) (crmcontracts.Deal, error) {
	if err := auth.Require(ctx, "deal", principal.ActionDelete); err != nil {
		return crmcontracts.Deal{}, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	var out crmcontracts.Deal
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := archiveDealInTx(ctx, tx, id, ifVersion, collectArchiveOptions(opts)); err != nil {
			return err
		}
		if out, err = readDealForCaller(ctx, tx, id, storekit.IncludeArchived, active); err != nil {
			return fmt.Errorf("read archived deal: %w", err)
		}
		return nil
	})
	return out, err
}

// ArchiveDealTx is ArchiveDeal on the caller's transaction, for a bulk change
// that archives many deals in one commit. It asks every gate ArchiveDeal asks
// and answers nothing: the caller already knows which row it archived.
func (s *Store) ArchiveDealTx(ctx context.Context, tx pgx.Tx, id ids.DealID, ifVersion *int64, opts ...ArchiveOption) error {
	if err := auth.Require(ctx, "deal", principal.ActionDelete); err != nil {
		return err
	}
	return archiveDealInTx(ctx, tx, id, ifVersion, collectArchiveOptions(opts))
}

// archiveDealInTx is the archive itself, behind the object gate its callers
// ask: the row's write check, the guarded patch and the cascade.
func archiveDealInTx(ctx context.Context, tx pgx.Tx, id ids.DealID, ifVersion *int64, o archiveOptions) error {
	// Locked first so the write check judges the row as the archive will find
	// it, not as it stood before a concurrent reassignment committed.
	if _, err := storekit.LockRow(ctx, tx, dealTable, id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	if err := auth.EnsureWritable(ctx, tx, dealTable, id.UUID); err != nil {
		return err
	}
	// Under the row lock the write takes, so a colleague's save lands wholly
	// before this read or after the commit and never between them.
	if err := refuseIfTouched(ctx, tx, id, o); err != nil {
		return err
	}
	// A liveness probe, not a wire read — no custom columns needed.
	if _, err := readDeal(ctx, tx, id, storekit.LiveOnly, nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	p := storekit.NewPatch()
	p.Set("archived_at", nil, now)
	if err := applyDealPatchGuarded(ctx, tx, id, p, ifVersion); err != nil {
		return fmt.Errorf("archive deal: %w", err)
	}
	// What the cascade retires and deletes is recorded on the archive's audit
	// row, which is what an un-archive reads to put it back.
	var cascade storekit.ArchiveCascade
	if err := cascade.Retire(ctx, tx, "relationship",
		`UPDATE relationship SET archived_at = @archived WHERE `+dealArchiveRelationships+` RETURNING id`,
		pgx.StrictNamedArgs{"record": id, "archived": now}); err != nil {
		return fmt.Errorf("archive the deal's relationships: %w", err)
	}
	if err := cascade.DropMemberships(ctx, tx,
		`WITH gone AS (
			DELETE FROM list_member WHERE `+dealArchiveMemberships+`
			RETURNING list_id, entity_type, entity_id, added_by, created_at, note),
		logged AS (
			INSERT INTO list_member_event (list_id, entity_type, entity_id, action, reason, actor)
			SELECT list_id, entity_type, entity_id, 'removed', 'record_archived', @actor FROM gone)
		SELECT list_id, added_by, created_at, note FROM gone`, id.UUID); err != nil {
		return fmt.Errorf("detach list memberships: %w", err)
	}
	if err := cascade.DropTags(ctx, tx,
		`DELETE FROM taggable WHERE `+dealArchiveMemberships+` RETURNING tag_id, assigned_by, assigned_by_kind, assigned_at`, pgx.StrictNamedArgs{"record": id.UUID}); err != nil {
		return fmt.Errorf("detach tags: %w", err)
	}

	auditID, err := storekit.AuditWithEvidence(ctx, tx, "archive", "deal", id.UUID, nil, nil, cascade.Evidence())
	if err != nil {
		return fmt.Errorf("audit deal archive: %w", err)
	}
	archived := crmcontracts.PublicEventDealArchived{}
	archived.Imported, archived.ImportRunId = storekit.ImportMark(ctx)
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, archived); err != nil {
		return fmt.Errorf("emit deal.archived: %w", err)
	}
	return nil
}

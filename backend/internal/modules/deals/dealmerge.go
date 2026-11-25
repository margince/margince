// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The two-deal merge. Facts about the opportunity move to the survivor and the
// path one deal took stays on the deal that took it: the stage-age report reads
// the most recent entry into a deal's current stage, so a loser's history
// copied onto the survivor would make the survivor look younger than it is.
// dealmergerelink.go holds the statements and says what each row does.

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CarryGrants moves the shares on a merged-away deal onto its survivor, inside
// the merge's transaction.
type CarryGrants func(ctx context.Context, tx pgx.Tx, recordType string, from, to ids.UUID) error

// MergeRefusedError maps to 409: the two deals cannot be merged as they stand,
// and Reason says what to change first.
type MergeRefusedError struct {
	Code    string
	Message string
}

func (e *MergeRefusedError) Error() string { return e.Message }

// The refusal codes a merge answers on target_id.
const (
	mergeRefusedCompany = "different_company"
	mergeRefusedHold    = "legal_hold"
	mergeRefusedBoth    = "both_hold_one"
)

// The audit keys a deal merge writes. The redirect pointer is recordmerge's, so
// all four merges spell it the same.
const (
	auditKeyDealMergedFrom = "merged_from_id"
	auditKeyDealFilled     = "filled"
)

// mergeTally counts what one merge moved, for the audit rows. Counts only,
// never the values.
type mergeTally struct {
	Offers        int64
	ActivityLinks int64
	Stakeholders  int64
	Contracts     int64
}

func (t mergeTally) audit() map[string]any {
	return map[string]any{
		"relinked_offers": t.Offers, "relinked_activity_links": t.ActivityLinks,
		"relinked_stakeholders": t.Stakeholders, "relinked_contracts": t.Contracts,
	}
}

// MergeDeal merges deal source into target and returns the survivor.
func (s *Store) MergeDeal(ctx context.Context, sourceID, targetID ids.DealID, ifVersion *int64) (crmcontracts.Deal, error) {
	// An absent target_id decodes to the zero id, which the self-merge guard
	// below does not catch.
	if err := httperr.RequireBodyID("target_id", targetID.UUID); err != nil {
		return crmcontracts.Deal{}, err
	}
	// The merge verb is an update of both rows: rewriting where records point is
	// curation, and recordmerge.Pair asks write authority on each end.
	if err := auth.Require(ctx, dealTable, principal.ActionUpdate); err != nil {
		return crmcontracts.Deal{}, err
	}
	if sourceID == targetID {
		return crmcontracts.Deal{}, &recordmerge.MergeSelfError{}
	}
	var out crmcontracts.Deal
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		active, err := s.activeColumnsIn(ctx, tx)
		if err != nil {
			return err
		}
		out, err = s.mergeDealTx(ctx, tx, sourceID, targetID, ifVersion, active)
		return err
	})
	return out, err
}

// mergeDealTx locks both ends in id order, resolves them, refuses a pair that
// cannot join, moves the references, fills the survivor's gaps, retires the
// source and writes the audit rows and the event. The pair lock is what makes
// the target check hold to commit: without it a concurrent merge of the target
// elsewhere would leave moved rows on a dead record.
func (s *Store) mergeDealTx(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.DealID, ifVersion *int64,
	active []fieldcatalog.Column,
) (crmcontracts.Deal, error) {
	_, tgtLock, err := storekit.LockPair(ctx, tx, dealTable, sourceID.UUID, targetID.UUID)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	src, tgt, err := recordmerge.Pair(ctx, tx, dealTable, sourceID, targetID, ifVersion, readDealMergeState)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	if err := refuseUnmergeablePair(ctx, tx, src, tgt); err != nil {
		return crmcontracts.Deal{}, err
	}
	tally, err := s.relinkDealReferences(ctx, tx, sourceID, targetID)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	p := buildDealSurvivorship(src, tgt)
	if err := settleDealMerge(ctx, tx, tgtLock, p, sourceID, targetID, tally); err != nil {
		return crmcontracts.Deal{}, err
	}
	out, err := s.readDealStamped(ctx, tx, targetID, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Deal{}, fmt.Errorf("read surviving deal: %w", err)
	}
	return out, nil
}

// readDealMergeState loads one end of a deal merge: a live row returns itself;
// an archived one returns its redirect pointer, nil when it was only archived.
func readDealMergeState(ctx context.Context, tx pgx.Tx, id ids.DealID) (crmcontracts.Deal, *ids.UUID, error) {
	d, err := readDeal(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return crmcontracts.Deal{}, nil, err
	}
	if d.ArchivedAt == nil {
		return d, nil, nil
	}
	return crmcontracts.Deal{}, (*ids.UUID)(d.MergedIntoId), apperrors.ErrNotFound
}

// refuseUnmergeablePair stops a merge whose rows could not join the survivor.
func refuseUnmergeablePair(ctx context.Context, tx pgx.Tx, src, tgt crmcontracts.Deal) error {
	// A contract's company must match its deal's, and a project must share the
	// company of every deal it holds.
	if !sameUUID(src.CompanyId, tgt.CompanyId) {
		return &MergeRefusedError{Code: mergeRefusedCompany,
			Message: "the two deals name different companies; merge the companies first"}
	}
	if src.LegalHold != nil && *src.LegalHold {
		return &MergeRefusedError{Code: mergeRefusedHold,
			Message: "this deal is under a legal hold; lift the hold before merging it away"}
	}
	return refuseDoubleHolders(ctx, tx, ids.DealID{UUID: ids.UUID(src.Id)}, ids.DealID{UUID: ids.UUID(tgt.Id)})
}

func sameUUID(a, b *openapi_types.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// buildDealSurvivorship fills the scalar columns the survivor holds nothing in
// from the source. Money is not among them: the
// survivor's price is its own, and a figure copied across would change it
// without anyone saying so.
func buildDealSurvivorship(src, tgt crmcontracts.Deal) *storekit.Patch {
	p := storekit.NewPatch()
	if tgt.Description == nil && src.Description != nil {
		p.Set("description", nil, *src.Description)
	}
	if tgt.AcquisitionSource == nil && src.AcquisitionSource != nil {
		p.Set("acquisition_source", nil, *src.AcquisitionSource)
	}
	if tgt.Priority == nil && src.Priority != nil {
		p.Set("priority", nil, string(*src.Priority))
	}
	if tgt.CommercialMotion == nil && src.CommercialMotion != nil {
		p.Set("commercial_motion", nil, string(*src.CommercialMotion))
	}
	// The partner and its attribution are one fact: a CHECK ties them.
	if tgt.PartnerCompanyId == nil && src.PartnerCompanyId != nil && src.PartnerAttribution != nil {
		p.Set("partner_company_id", nil, ids.UUID(*src.PartnerCompanyId))
		p.Set("partner_attribution", nil, string(*src.PartnerAttribution))
	}
	return p
}

// settleDealMerge applies the survivorship fill, retires the source and writes
// the audit rows and the event.
//
//gate:parent-version-door
func settleDealMerge(ctx context.Context, tx pgx.Tx, tgtLock storekit.RowLock, p *storekit.Patch,
	sourceID, targetID ids.DealID, tally mergeTally,
) error {
	if !p.Empty() {
		if err := applyDealPatchLocked(ctx, tx, p, tgtLock); err != nil {
			return fmt.Errorf("apply survivorship fill: %w", err)
		}
	} else if err := storekit.TouchLocked(ctx, tx, tgtLock); err != nil {
		return fmt.Errorf("move the survivor's version: %w", err)
	}
	if err := recordmerge.ArchiveMergedAway(ctx, tx, dealTable, sourceID.UUID, targetID.UUID); err != nil {
		return fmt.Errorf("retire merged-away deal: %w", err)
	}
	filled := slices.Sorted(maps.Keys(p.After()))
	before := map[string]any{recordmerge.MergedIntoKey: nil}
	after := map[string]any{recordmerge.MergedIntoKey: targetID, auditKeyDealFilled: filled}
	maps.Copy(after, tally.audit())
	auditID, err := storekit.Audit(ctx, tx, "merge", dealTable, sourceID.UUID, before, after)
	if err != nil {
		return fmt.Errorf("audit deal merge: %w", err)
	}
	if err := EmitDealMerged(ctx, tx, auditID, sourceID, targetID); err != nil {
		return fmt.Errorf("emit deal.merged: %w", err)
	}
	// Each filled column is its own top-level key, as in an update row, so the
	// history read masks it by key.
	survivorBefore := map[string]any{auditKeyDealMergedFrom: nil}
	survivorAfter := map[string]any{auditKeyDealMergedFrom: sourceID.UUID}
	maps.Copy(survivorAfter, tally.audit())
	maps.Copy(survivorBefore, p.Before())
	maps.Copy(survivorAfter, p.After())
	if _, err := storekit.Audit(ctx, tx, "merge", dealTable, targetID.UUID, survivorBefore, survivorAfter); err != nil {
		return fmt.Errorf("audit deal merge on the survivor: %w", err)
	}
	return nil
}

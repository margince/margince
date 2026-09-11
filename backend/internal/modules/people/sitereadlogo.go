// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The marks a cold-start dossier holds for a company that does not exist yet.
//
// An onboarding read is unbound by construction — it reads the site to propose
// a company a human has not confirmed into being — and the seed page's
// declarations are in hand only while the crawl is, so a mark that is not
// parked here is a mark nothing can resolve later. The dossier parks one per
// SLOT, in the same pair of columns per slot the company row carries
// (companylogowrite.go), and the confirmation moves each onto the record
// (bindSiteReadLogo). Both slots take one statement each, with the slot as a
// bind parameter, for the reason the company write gives: two spellings
// of one UPDATE agree right up until one of them is edited.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// siteReadLogoWrite parks one slot's mark on a dossier the caller still holds.
// The RETURNING is the pre-write key, exactly as companyLogoWrite's is: the
// sub-select reads the statement's own snapshot, so it names the object this
// write supersedes rather than the one it just stored. The casts are what let
// Postgres type the CASE arms from bound parameters.
const siteReadLogoWrite = `UPDATE site_read SET
		logo_object_key      = CASE WHEN $5::boolean THEN $2::text ELSE logo_object_key END,
		logo_origin          = CASE WHEN $5::boolean THEN $3::text ELSE logo_origin END,
		logo_icon_object_key = CASE WHEN $5::boolean THEN logo_icon_object_key ELSE $2::text END,
		logo_icon_origin     = CASE WHEN $5::boolean THEN logo_icon_origin ELSE $3::text END,
		updated_at = now()
	WHERE id = $1 AND status = 'running' AND started_at = $4
	  AND company_id IS NULL AND confirmed_at IS NULL
	RETURNING (SELECT CASE WHEN $5::boolean THEN sr.logo_object_key ELSE sr.logo_icon_object_key END
	             FROM site_read sr WHERE sr.id = $1)`

// siteReadLogoSlotKey names one slot's parked key inside a statement that
// aliases site_read as `sr` and binds the slot as $2.
const siteReadLogoSlotKey = `CASE WHEN $2::boolean THEN sr.logo_object_key ELSE sr.logo_icon_object_key END`

// logoUnwornByAnyCompany is the safety proof every drop of a parked
// reference carries, in one spelling: the reference goes only while no
// company names the same key in EITHER of its slots, so an object a
// company wears is never reported as collectable. The key is the whole scope
// and it is enough: an object key carries its workspace prefix, so two tenants
// cannot name the same object and a foreign record's bytes cannot be described
// here at all. Said plainly here because no row-level policy says it a second
// way. It reads the `sr` alias and the $2 slot its statements bind.
const logoUnwornByAnyCompany = `NOT EXISTS (
		SELECT 1 FROM company o
		WHERE o.logo_object_key = ` + siteReadLogoSlotKey + `
		   OR o.logo_icon_object_key = ` + siteReadLogoSlotKey + `)`

// siteReadLogoRelease drops one slot's parked reference and answers the key it
// dropped — the pre-write key through the statement's own snapshot, as every
// write in this lane reports its collection. Callers append the condition that
// says WHEN a reference may go; the unworn proof is always part of it.
const siteReadLogoRelease = `UPDATE site_read sr SET
		logo_object_key      = CASE WHEN $2::boolean THEN NULL ELSE logo_object_key END,
		logo_origin          = CASE WHEN $2::boolean THEN NULL ELSE logo_origin END,
		logo_icon_object_key = CASE WHEN $2::boolean THEN logo_icon_object_key ELSE NULL END,
		logo_icon_origin     = CASE WHEN $2::boolean THEN logo_icon_origin ELSE NULL END,
		updated_at = now()
	WHERE sr.id = $1 AND ` + siteReadLogoSlotKey + ` IS NOT NULL
	  AND ` + logoUnwornByAnyCompany

// siteReadLogoReleased is the RETURNING every release ends with.
const siteReadLogoReleased = ` RETURNING (SELECT CASE WHEN $2::boolean THEN parked.logo_object_key ELSE parked.logo_icon_object_key END
	             FROM site_read parked WHERE parked.id = $1)`

// logoSlots lists the slots a dossier parks, in the order a confirmation binds
// them: the wide mark first, because it is the one every record can wear and
// the one the collapsed rail falls back to.
var logoSlots = []LogoSlot{LogoWide, LogoIcon}

// SiteReadLogoKey reports where the WIDE mark an onboarding read resolved is
// stored, so the review can show the company it is about before a confirmation
// binds the mark to a record. ErrNotFound for a read that resolved none as much
// as for one that does not exist: the client draws the monogram for both, and
// telling them apart would say which dossiers exist.
//
// The badge is not served from the dossier: the review shows the company's
// face once, and the confirmed profile carries the badge from then on.
//
// The same gate GetCompanySiteRead holds: the reader of a dossier is the one
// who may read the record it will become, or create it on the cold start.
func (s *Store) SiteReadLogoKey(ctx context.Context, readID ids.UUID) (string, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		if createErr := auth.Require(ctx, "company", principal.ActionCreate); createErr != nil {
			return "", createErr
		}
	}
	var key *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT logo_object_key FROM site_read WHERE id = $1 AND target_kind = 'onboarding'`, readID,
		).Scan(&key)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("site read %s logo: %w", readID, err)
	}
	if key == nil || *key == "" {
		return "", apperrors.ErrNotFound
	}
	return *key, nil
}

// RecordSiteReadLogo parks a resolved mark on the dossier that found it, in the
// slot the caller names, for a read whose subject does not exist yet.
//
// Only the attempt that currently HOLDS the read takes the reference: the read
// must still be running AND still carry the lease the caller was handed at
// BeginSiteRead (SiteReadClaim.ClaimedAt). Running alone is not enough, because
// a reclaim puts a running row back into running under a NEW attempt: a stalled
// worker resuming afterwards would overwrite the mark the current attempt just
// parked and be handed that attempt's object back as superseded, so its caller
// would delete the very bytes the dossier had adopted.
//
// Past the claim the row has also already answered for its marks: a read that
// ended without a company had its parked objects collected on the way to
// terminal, and a read that ended with a report handed a human a draft whose
// face must not change while they review it. A park after either is a reference
// nothing adopts and nothing collects, which is the one orphan this lane can no
// longer find.
//
// It reports whether the dossier took the reference and hands back the key the
// slot named before, so the caller can reclaim bytes nothing references any
// more. A refused park hands back none — the object stored for it is the
// caller's to collect. Same contract as SetCompanyLogo, for the same
// reason: each attempt writes its own key, so two resolves of one read can never
// write the same object.
//
// No auth.Require, the same rationale as BeginSiteRead: the worker is not a
// human principal, the human's authority was checked when the read was started,
// and the workspace-bound transaction still scopes the write to the job's
// tenant. The reference on the dossier is operational, like every other worker
// write to this row; the audited write on the RECORD happens when a
// confirmation binds it.
func (s *Store) RecordSiteReadLogo(ctx context.Context, readID ids.UUID, claimedAt time.Time, slot LogoSlot, objectKey, originURL string) (recorded bool, supersededKey *string, err error) {
	if objectKey == "" || originURL == "" {
		return false, nil, errors.New("people: a resolved logo needs both its storage key and the URL it was resolved from")
	}
	if claimedAt.IsZero() {
		return false, nil, errors.New("people: parking a website read's mark needs the lease BeginSiteRead handed the attempt that resolved it")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var previous *string
		err := tx.QueryRow(ctx, siteReadLogoWrite,
			readID, objectKey, originURL, claimedAt.UTC(), slot.wide()).Scan(&previous)
		if errors.Is(err, pgx.ErrNoRows) {
			// Bound, confirmed, no longer running, or running for somebody else:
			// the read has answered for its marks without this one, and recording
			// it now would name bytes no record adopts and no collection reaches.
			return nil
		}
		if err != nil {
			return fmt.Errorf("record the website read's %s: %w", slot, err)
		}
		recorded = true
		supersededKey = supersededObject(previous, objectKey)
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return recorded, supersededKey, nil
}

// DiscardSiteReadLogo drops the marks parked on a read that can never hand
// them over, and answers the storage keys it dropped so the caller collects the
// bytes — this module owns no object store, so reclaiming is something it can
// only ever REPORT.
//
// A confirmation is the only thing that adopts a parked mark, and it accepts
// none but a done or partial read. A read that ended without a dossier —
// failed, or cancelled because the operator withdrew the setting that queued it
// — therefore holds bytes no record will ever wear, and the reference on the
// dossier is the only thing left that can still find them.
//
// No auth.Require, the same rationale as RecordSiteReadLogo: the worker is not
// a human principal, and the parked reference is operational state on the
// dossier row rather than a fact on a record.
func (s *Store) DiscardSiteReadLogo(ctx context.Context, readID ids.UUID) ([]string, error) {
	var discarded []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		for _, slot := range logoSlots {
			var key *string
			err := tx.QueryRow(ctx, siteReadLogoRelease+`
			  AND sr.confirmed_at IS NULL AND sr.status IN ('failed', 'cancelled')`+siteReadLogoReleased,
				readID, slot.wide()).Scan(&key)
			if errors.Is(err, pgx.ErrNoRows) {
				// Nothing parked in this slot, a read that may still be confirmed,
				// or bytes a record already wears: all three keep the object.
				continue
			}
			if err != nil {
				return fmt.Errorf("discard the website read's %s: %w", slot, err)
			}
			if key != nil && *key != "" {
				discarded = append(discarded, *key)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return discarded, nil
}

// releaseParkedSiteReadLogo drops the reference a confirmation declined to
// adopt in one slot and answers the key it dropped, so the caller collects
// bytes no record wears. The confirmation holds this dossier's row lock, so the
// drop and the answer describe one state.
//
// A caller that owns no object store asks for no release and the reference
// STAYS. The parked key embeds a per-attempt uuid nothing else recorded — it is
// the object's last handle — so clearing it for a caller that cannot delete
// would turn a findable orphan into an unfindable one. It is the confirm path's
// spelling of the guard the worker keeps in front of DiscardSiteReadLogo.
func releaseParkedSiteReadLogo(ctx context.Context, tx pgx.Tx, readID ids.UUID, slot LogoSlot, reclaim bool) (*string, error) {
	// released stays nil for the two outcomes that free nothing: a caller that
	// cannot collect, and bytes a record already wears — the statement matches no
	// row then, and the reference stays with the record.
	var released *string
	if !reclaim {
		return released, nil
	}
	err := tx.QueryRow(ctx, siteReadLogoRelease+siteReadLogoReleased, readID, slot.wide()).Scan(&released)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("release the website read's unadopted %s: %w", slot, err)
	}
	return released, nil
}

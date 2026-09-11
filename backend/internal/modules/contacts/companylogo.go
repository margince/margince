// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The company's visual identity (A55): the row holds a reference to the
// normalized bytes in object storage plus the page URL they were resolved
// from, never the bytes themselves. A logo is a DISPLAY asset, so resolving
// one is a 🟢 write that needs no confirm — but it obeys the same
// human-precedence rule every enriched field does: a mark a human uploaded
// is never replaced by one a machine found, and a resolve that meets one
// leaves it alone rather than staging a change nobody asked for.

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetCompanyLogo records a resolved company mark in the WIDE slot: the
// storage key its normalized bytes live at, and the asset URL it came from.
// It names that slot rather than taking one as an argument, because an
// enrichment read resolves ONE picture of a company — the square-preferring
// mark every record card draws — and only the cold-start dossier resolves a
// badge beside it (RecordSiteReadLogo).
//
// It reports whether the row was written — false means a human's own logo holds
// the field, which is a normal outcome and not an error — and hands back the key
// the row named BEFORE this write, so the caller can reclaim bytes nothing
// references any more.
//
// The bytes must already be stored: this store is blob-free, the same division
// the offer PDF's asset ref keeps, so a caller writes the object first and the
// reference second.
//
// The key a caller passes must be unique to its own attempt, and the returned
// one is how the superseded object gets collected. A key derived from the
// company alone would make two concurrent resolves write the SAME object:
// each would overwrite the other's bytes while the row recorded whichever
// transaction committed last, leaving the stored image and the origin URL
// describing different pictures.
func (s *Store) SetCompanyLogo(ctx context.Context, id ids.CompanyID, objectKey, originURL string) (written bool, supersededKey *string, err error) {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return false, nil, err
	}
	if objectKey == "" || originURL == "" {
		return false, nil, errors.New("contacts: a resolved logo needs both its storage key and the URL it was resolved from")
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The target is a KNOWN row, so row-scope is re-checked here: a leaked
		// company id buys nothing (existence-hiding 404).
		if err := auth.EnsureWritable(ctx, tx, "company", id.UUID); err != nil {
			return err
		}
		// Lock the row before reading who holds the field. The guard is a read
		// followed by a write, so without the lock a contact's upload landing
		// between the two would be read as absent and then overwritten — the
		// precedence rule would hold on every run except the one where it
		// matters. Any writer of this company takes the same lock, so the
		// two serialize instead of racing.
		// Live rows only, matching every other mutation and CompanyLogoKey:
		// a lock that admitted tombstones would let an archived company
		// reach the human-precedence return and answer "no change" where the
		// rest of the module answers not-found.
		var locked ids.UUID
		err := tx.QueryRow(ctx,
			`SELECT id FROM company WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock company for the logo write: %w", err)
		}
		held, err := logoHeldByHuman(ctx, tx, id, LogoWide)
		if err != nil {
			return err
		}
		if held {
			return nil
		}
		// RETURNING the pre-write key: this transaction holds the row lock, so
		// it is the one place that can hand back the object its own write
		// supersedes. Reading it separately afterwards would name whatever the
		// NEXT resolve had since put there.
		var previous, previousOrigin *string
		err = tx.QueryRow(ctx, companyLogoWrite,
			id, objectKey, originURL, LogoWide.wide()).Scan(&previous, &previousOrigin)
		if errors.Is(err, pgx.ErrNoRows) {
			// Visible above but not updatable here: the row was archived
			// between the two statements. Nothing to record.
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("set company logo: %w", err)
		}
		written = true
		supersededKey = supersededObject(previous, objectKey)
		return recordLogoWrite(ctx, tx, id, LogoWide, resolvedLogoWrite(previousOrigin, originURL, by))
	})
	if err != nil {
		return false, nil, err
	}
	return written, supersededKey, nil
}

// bindSiteReadLogo gives the company one of the marks its own website read
// resolved — the step that makes the anchor's face arrive on the same terms as
// every other company's (A55). It runs inside the confirmation's transaction,
// once per slot, so the company and its marks commit together.
//
// One thing outranks it, and only one: a mark a CONTACT chose for that slot.
// Everything else the read resolves lands, including over a mark an earlier
// read landed, because the same read can be run again against a company that
// already exists — and a re-read asked for precisely to pick up a new logo that
// then declined to write it would be a verb that reports success and does
// nothing.
//
// Adoption MOVES the dossier's reference onto the record; a decline hands that
// reference back to the caller as a key to collect, in the same transaction that
// declined, so the row and the report can never disagree about who holds those
// bytes. An adoption that supersedes an older mark hands THAT key back for the
// same reason.
//
// It answers the key, never a deleted object: this module owns no object store,
// so reclaiming is something it can only ever REPORT.
func bindSiteReadLogo(ctx context.Context, tx pgx.Tx, readID ids.UUID, companyID ids.CompanyID, slot LogoSlot, reclaim bool) (*string, error) {
	// unadopted names what the confirmation leaves behind for its caller. It
	// stays nil on every path that leaves nothing: no mark was parked, or the
	// anchor took the one that was and wore none before it.
	var unadopted *string
	var objectKey, originURL *string
	if err := tx.QueryRow(ctx, `SELECT `+siteReadLogoSlotKey+`,
		CASE WHEN $2::boolean THEN sr.logo_origin ELSE sr.logo_icon_origin END
		FROM site_read sr WHERE sr.id = $1`, readID, slot.wide()).
		Scan(&objectKey, &originURL); err != nil {
		return nil, fmt.Errorf("read the website read's %s: %w", slot, err)
	}
	if objectKey == nil || *objectKey == "" || originURL == nil || *originURL == "" {
		// The read resolved nothing usable for this slot — an air-gapped
		// install, a site that declares no icon, an asset that would not decode.
		// The record draws its deterministic monogram, or the collapsed rail
		// draws the wide mark, which is a face rather than a gap either way.
		return unadopted, nil
	}
	held, err := logoHeldByHuman(ctx, tx, companyID, slot)
	if err != nil {
		return nil, err
	}
	if held {
		return releaseParkedSiteReadLogo(ctx, tx, readID, slot, reclaim)
	}
	// Not fill-empty. A confirmation is not only the step that CREATES the
	// company: the same read can be run again against a company that already
	// exists, to pick up what its website says now, and a re-read whose whole
	// point is a fresher mark that then declined to write it would leave the
	// company wearing the picture its site stopped using. What must not be
	// overwritten is a mark a CONTACT chose, and that is the guard above.
	//
	// The record's own previous object comes back with it, because adopting a
	// new mark is what makes the old one unreferenced — and the caller is the
	// only side that can collect bytes.
	var previousKey, previousOrigin *string
	err = tx.QueryRow(ctx, companyLogoWrite,
		companyID, *objectKey, *originURL, slot.wide()).Scan(&previousKey, &previousOrigin)
	if errors.Is(err, pgx.ErrNoRows) {
		// Archived under this confirmation: nothing to wear a mark, and the
		// same parked object left over.
		return releaseParkedSiteReadLogo(ctx, tx, readID, slot, reclaim)
	}
	if err != nil {
		return nil, fmt.Errorf("bind the website read's %s: %w", slot, err)
	}
	// Handed over, not shared: the record is the object's one reference now.
	// Two rows naming one key would let a later resolve of this company
	// supersede it, collect the bytes, and leave the dossier pointing at an
	// object nothing can serve. The dossier keeps its reference only while
	// NOTHING else holds it — that is what makes an unadopted mark findable,
	// and the reason this clears only on the run that actually adopted.
	if _, err := tx.Exec(ctx, `UPDATE site_read SET
		logo_object_key      = CASE WHEN $2::boolean THEN NULL ELSE logo_object_key END,
		logo_origin          = CASE WHEN $2::boolean THEN NULL ELSE logo_origin END,
		logo_icon_object_key = CASE WHEN $2::boolean THEN logo_icon_object_key ELSE NULL END,
		logo_icon_origin     = CASE WHEN $2::boolean THEN logo_icon_origin ELSE NULL END,
		updated_at = now()
		WHERE id = $1`, readID, slot.wide()); err != nil {
		return nil, fmt.Errorf("hand the website read's %s to the company: %w", slot, err)
	}
	// The site read is what captured this, never the human who confirmed the
	// draft: provenance is written once and never re-derived, and a machine mark
	// recorded under a contact's name would make the human-precedence guard
	// refuse every later resolve for a logo nobody chose.
	if err := recordLogoWrite(ctx, tx, companyID, slot,
		resolvedLogoWrite(previousOrigin, *originURL, companySiteReadCapturedBy)); err != nil {
		return nil, err
	}
	// What this confirmation left unreferenced: the mark the record wore before
	// it, if it wore one. On the confirmation that CREATES the company there is
	// none, which is why this stays nil for every onboarding read.
	return supersededObject(previousKey, *objectKey), nil
}

// supersededObject names the object this write orphaned, or nil when it
// orphaned none — the company had no logo, or the caller re-recorded the
// key already on the row.
func supersededObject(previous *string, objectKey string) *string {
	if previous == nil || *previous == "" || *previous == objectKey {
		return nil
	}
	return previous
}

// LogoHeldByHuman answers whether a contact set this company's WIDE mark,
// for a caller that must know BEFORE it does expensive or irreversible work —
// the
// site read asks first so it neither fetches a logo it may not use nor
// overwrites the object a contact's own logo already occupies. It carries the
// record's read gate, so a company the caller cannot see is not found.
//
// The write applies the same rule again under a row lock: this read is an
// optimization and a byte-safety check, never the authority.
func (s *Store) LogoHeldByHuman(ctx context.Context, id ids.CompanyID) (bool, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return false, err
	}
	var held bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "company", id.UUID); err != nil {
			return err
		}
		var err error
		held, err = logoHeldByHuman(ctx, tx, id, LogoWide)
		return err
	})
	return held, err
}

// logoHeldByHuman reports whether a contact's own mark is on this company
// in one slot right now.
//
// TWO questions, because the logo's provenance can outlive the logo. What holds
// a read off is a mark a contact chose, not the fact that a contact once touched
// the field: someone who REMOVES a logo has asked for the record to have none,
// and leaving their removal standing as a hold would mean the company could
// never be given a face again by any later read. So the field's own answer —
// companyFieldHeldByHuman, shared with the description — is narrowed by whether
// there is a mark in the slot at all. The description needs no such arm: its
// column and its provenance move together.
func logoHeldByHuman(ctx context.Context, tx pgx.Tx, id ids.CompanyID, slot LogoSlot) (bool, error) {
	human, err := companyFieldHeldByHuman(ctx, tx, id, slot.field())
	if err != nil || !human {
		return false, err
	}
	var wearsMark bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM company WHERE id = $1
			AND CASE WHEN $2::boolean THEN logo_object_key ELSE logo_icon_object_key END IS NOT NULL)`,
		id, slot.wide()).Scan(&wearsMark); err != nil {
		return false, fmt.Errorf("read whether the company wears a %s: %w", slot, err)
	}
	return wearsMark, nil
}

// CompanyLogoKey answers where one slot's logo bytes live, for a caller
// that streams them. It returns ErrNotFound both when the company is
// invisible or absent and when it simply wears no mark in that slot: to the
// client those are the same answer — draw the monogram — and distinguishing
// them would leak which companies exist.
func (s *Store) CompanyLogoKey(ctx context.Context, id ids.CompanyID, slot LogoSlot) (string, error) {
	// A logo is part of the record, so reading its location is a read of the
	// record and carries the record's gate.
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return "", err
	}
	var key *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "company", id.UUID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, companyLogoKeyRead, id, slot.wide()).Scan(&key)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if key == nil || *key == "" {
		return "", apperrors.ErrNotFound
	}
	return *key, nil
}

// logoRevisionDigest names one slot's stored bytes on the wire: LogoURL bakes
// it into a cache-busting query token, and streamLogo answers with the same
// value as an ETag — one spelling, so the two mechanisms naming the same
// bytes can never drift into disagreeing about which revision they mean.
//
// The prefix versions the representation as well as the object. Version 2
// removes the transparent square canvas written by older logo uploads, so a
// browser that cached that letterboxed response must fetch the wide one. The
// slot needs no version of its own: each slot has its own path, and a key is
// minted per upload (companyLogoKey / siteReadLogoKey,
// compose/sitelogo.go), so two marks can never share a digest.
func logoRevisionDigest(objectKey string) string {
	digest := sha256.Sum256([]byte("logo-display-v2\x00" + objectKey))
	return fmt.Sprintf("%x", digest[:6])
}

// LogoURL renders where a client fetches one slot's logo bytes, or nil when the
// company wears no mark there. Its query token changes with the object key,
// so replacing a logo cannot leave a browser showing the previous cached image
// at the same URL. The key itself never reaches the wire: it names a bucket
// path, and only a short one-way digest is exposed.
//
// Exported because the account-graph assembly reads company rows of its
// own and must spell this URL exactly as this module's own reads do — one
// spelling, or a company's face differs between its record and the graph.
func LogoURL(id ids.UUID, objectKey *string, slot LogoSlot) *string {
	if objectKey == nil || *objectKey == "" {
		return nil
	}
	path := fmt.Sprintf("/v1/companies/%s/logo%s?v=%s", id.String(), slot.urlSuffix(), logoRevisionDigest(*objectKey))
	return &path
}

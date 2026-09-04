// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The organization's visual identity (A55): the row holds a reference to the
// normalized bytes in object storage plus the page URL they were resolved
// from, never the bytes themselves. A logo is a DISPLAY asset, so resolving
// one is a 🟢 write that needs no confirm — but it obeys the same
// human-precedence rule every enriched field does: a mark a person uploaded
// is never replaced by one a machine found, and a resolve that meets one
// leaves it alone rather than staging a change nobody asked for.

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// logoFieldName is how the logo is spelled in field_provenance and in the
// audit/event delta. One spelling, so the provenance display and the write
// cannot disagree about which field was set.
// Held by: TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed (backend/gates/claimedspelling_test.go)
const logoFieldName = "logo"

// SetOrganizationLogo records a resolved company mark: the storage key its
// normalized bytes live at, and the asset URL it came from. It reports whether
// the row was written — false means a human's own logo holds the field, which
// is a normal outcome and not an error — and hands back the key the row named
// BEFORE this write, so the caller can reclaim bytes nothing references any
// more.
//
// The bytes must already be stored: this store is blob-free, the same division
// the offer PDF's asset ref keeps, so a caller writes the object first and the
// reference second.
//
// The key a caller passes must be unique to its own attempt, and the returned
// one is how the superseded object gets collected. A key derived from the
// organization alone would make two concurrent resolves write the SAME object:
// each would overwrite the other's bytes while the row recorded whichever
// transaction committed last, leaving the stored image and the origin URL
// describing different pictures.
func (s *Store) SetOrganizationLogo(ctx context.Context, id ids.OrganizationID, objectKey, originURL string) (written bool, supersededKey *string, err error) {
	if err := auth.Require(ctx, "organization", principal.ActionUpdate); err != nil {
		return false, nil, err
	}
	if objectKey == "" || originURL == "" {
		return false, nil, errors.New("people: a resolved logo needs both its storage key and the URL it was resolved from")
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The target is a KNOWN row, so row-scope is re-checked here: a leaked
		// organization id buys nothing (existence-hiding 404).
		if err := auth.EnsureWritable(ctx, tx, "organization", id.UUID); err != nil {
			return err
		}
		// Lock the row before reading who holds the field. The guard is a read
		// followed by a write, so without the lock a person's upload landing
		// between the two would be read as absent and then overwritten — the
		// precedence rule would hold on every run except the one where it
		// matters. Any writer of this organization takes the same lock, so the
		// two serialize instead of racing.
		// Live rows only, matching every other mutation and OrganizationLogoKey:
		// a lock that admitted tombstones would let an archived organization
		// reach the human-precedence return and answer "no change" where the
		// rest of the module answers not-found.
		var locked ids.UUID
		err := tx.QueryRow(ctx,
			`SELECT id FROM organization WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock organization for the logo write: %w", err)
		}
		held, err := logoHeldByHuman(ctx, tx, id)
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
		err = tx.QueryRow(ctx, `
			UPDATE organization SET logo_object_key = $2, logo_origin = $3
			WHERE id = $1 AND archived_at IS NULL
			RETURNING (SELECT o.logo_object_key FROM organization o WHERE o.id = $1),
			          (SELECT o.logo_origin FROM organization o WHERE o.id = $1)`,
			id, objectKey, originURL).Scan(&previous, &previousOrigin)
		if errors.Is(err, pgx.ErrNoRows) {
			// Visible above but not updatable here: the row was archived
			// between the two statements. Nothing to record.
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("set organization logo: %w", err)
		}
		written = true
		supersededKey = supersededObject(previous, objectKey)
		return recordLogoWrite(ctx, tx, id, resolvedLogoWrite(previousOrigin, originURL, by))
	})
	if err != nil {
		return false, nil, err
	}
	return written, supersededKey, nil
}

// RecordSiteReadLogo parks a resolved mark on the dossier that found it, for a
// read whose subject does not exist yet. An onboarding read is unbound by
// construction — it reads the site to propose a company a human has not
// confirmed into being — and the seed page's declarations are in hand only
// while the crawl is, so a mark that is not parked here is a mark nothing can
// resolve later.
//
// Only the attempt that currently HOLDS the read takes the reference: the read
// must still be running AND still carry the lease the caller was handed at
// BeginSiteRead (SiteReadClaim.ClaimedAt). Running alone is not enough, because
// a reclaim puts a running row back into running under a NEW attempt: a stalled
// worker resuming afterwards would overwrite the mark the current attempt just
// parked and be handed that attempt's object back as superseded, so its caller
// would delete the very bytes the dossier had adopted.
//
// Past the claim the row has also already answered for its mark: a read that
// ended without a company had its parked object collected on the way to
// terminal, and a read that ended with a report handed a human a draft whose
// mark must not change while they review it. A park after either is a reference
// nothing adopts and nothing collects, which is the one orphan this lane can no
// longer find.
//
// It reports whether the dossier took the reference and hands back the key the
// row named before, so the caller can reclaim bytes nothing references any more.
// A refused park hands back none — the object stored for it is the caller's to
// collect. Same contract as SetOrganizationLogo, for the same reason: each
// attempt writes its own key, so two resolves of one read can never write the
// same object.
//
// No auth.Require, the same rationale as BeginSiteRead: the worker is not a
// human principal, the human's authority was checked when the read was started,
// and the workspace-bound transaction still scopes the write to the job's
// tenant. The reference on the dossier is operational, like every other worker
// write to this row; the audited write on the RECORD happens when a
// confirmation binds it.
func (s *Store) RecordSiteReadLogo(ctx context.Context, readID ids.UUID, claimedAt time.Time, objectKey, originURL string) (recorded bool, supersededKey *string, err error) {
	if objectKey == "" || originURL == "" {
		return false, nil, errors.New("people: a resolved logo needs both its storage key and the URL it was resolved from")
	}
	if claimedAt.IsZero() {
		return false, nil, errors.New("people: parking a website read's mark needs the lease BeginSiteRead handed the attempt that resolved it")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// RETURNING the pre-write key, exactly as SetOrganizationLogo does: the
		// sub-select reads the statement's own snapshot, so it names the object
		// this write supersedes rather than the one it just stored.
		var previous *string
		err := tx.QueryRow(ctx, `
			UPDATE site_read SET logo_object_key = $2, logo_origin = $3, updated_at = now()
			WHERE id = $1 AND status = 'running' AND started_at = $4
			  AND organization_id IS NULL AND confirmed_at IS NULL
			RETURNING (SELECT sr.logo_object_key FROM site_read sr WHERE sr.id = $1)`,
			readID, objectKey, originURL, claimedAt.UTC()).Scan(&previous)
		if errors.Is(err, pgx.ErrNoRows) {
			// Bound, confirmed, no longer running, or running for somebody else:
			// the read has answered for its mark without this one, and recording
			// it now would name bytes no record adopts and no collection reaches.
			return nil
		}
		if err != nil {
			return fmt.Errorf("record the website read's logo: %w", err)
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

// logoUnwornByAnyOrganization is the safety proof every drop of a parked
// reference carries, in one spelling: the reference goes only while no
// organization names the same key, so an object a company wears is never
// reported as collectable. The key is the whole scope and it is enough: an
// object key carries its workspace prefix, so two tenants cannot name the
// same object and a foreign record's bytes cannot be described here at all.
// Said plainly because core 0217 retired the policy that used to say it a
// second way. It reads the `sr` alias its statements give site_read.
const logoUnwornByAnyOrganization = `NOT EXISTS (
		SELECT 1 FROM organization o WHERE o.logo_object_key = sr.logo_object_key)`

// DiscardSiteReadLogo drops the mark parked on a read that can never hand it
// over, and answers the storage key it dropped so the caller collects the
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
func (s *Store) DiscardSiteReadLogo(ctx context.Context, readID ids.UUID) (*string, error) {
	var discarded *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// RETURNING the pre-write key, exactly as the two writes above do: the
		// sub-select reads the statement's own snapshot, so it names the object
		// this UPDATE just unreferenced.
		err := tx.QueryRow(ctx, `
			UPDATE site_read sr SET logo_object_key = NULL, logo_origin = NULL, updated_at = now()
			WHERE sr.id = $1 AND sr.confirmed_at IS NULL AND sr.logo_object_key IS NOT NULL
			  AND sr.status IN ('failed', 'cancelled')
			  AND `+logoUnwornByAnyOrganization+`
			RETURNING (SELECT parked.logo_object_key FROM site_read parked WHERE parked.id = $1)`,
			readID).Scan(&discarded)
		if errors.Is(err, pgx.ErrNoRows) {
			// Nothing parked, a read that may still be confirmed, or bytes a
			// record already wears: all three keep the object.
			return nil
		}
		if err != nil {
			return fmt.Errorf("discard the website read's logo: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return discarded, nil
}

// bindSiteReadLogo gives the company the mark its own website read resolved —
// the step that makes the anchor's face arrive on the same terms as every other
// company's (A55). It runs inside the confirmation's transaction, so the
// company and its logo commit together.
//
// One thing outranks it, and only one: a mark a PERSON chose. Everything else
// the read resolves lands, including over a mark an earlier read landed, because
// the same read can be run again against a company that already exists — and a
// re-read asked for precisely to pick up a new logo that then declined to write
// it would be a verb that reports success and does nothing.
//
// Adoption MOVES the dossier's reference onto the record; a decline hands that
// reference back to the caller as a key to collect, in the same transaction that
// declined, so the row and the report can never disagree about who holds those
// bytes. An adoption that supersedes an older mark hands THAT key back for the
// same reason.
//
// It answers the key, never a deleted object: this module owns no object store,
// so reclaiming is something it can only ever REPORT.
func bindSiteReadLogo(ctx context.Context, tx pgx.Tx, readID ids.UUID, orgID ids.OrganizationID, reclaim bool) (*string, error) {
	// unadopted names what the confirmation leaves behind for its caller. It
	// stays nil on every path that leaves nothing: no mark was parked, or the
	// anchor took the one that was and wore none before it.
	var unadopted *string
	var objectKey, originURL *string
	if err := tx.QueryRow(ctx,
		`SELECT logo_object_key, logo_origin FROM site_read WHERE id = $1`, readID).
		Scan(&objectKey, &originURL); err != nil {
		return nil, fmt.Errorf("read the website read's logo: %w", err)
	}
	if objectKey == nil || *objectKey == "" || originURL == nil || *originURL == "" {
		// The read resolved nothing usable — an air-gapped install, a site that
		// declares no icon, an asset that would not decode. The record draws its
		// deterministic monogram, which is a face rather than a gap.
		return unadopted, nil
	}
	held, err := logoHeldByHuman(ctx, tx, orgID)
	if err != nil {
		return nil, err
	}
	if held {
		return releaseParkedSiteReadLogo(ctx, tx, readID, reclaim)
	}
	// Not fill-empty. A confirmation is not only the step that CREATES the
	// company: the same read can be run again against a company that already
	// exists, to pick up what its website says now, and a re-read whose whole
	// point is a fresher mark that then declined to write it would leave the
	// company wearing the picture its site stopped using. What must not be
	// overwritten is a mark a PERSON chose, and that is the guard above.
	//
	// The record's own previous object comes back with it, because adopting a
	// new mark is what makes the old one unreferenced — and the caller is the
	// only side that can collect bytes.
	var previousKey, previousOrigin *string
	err = tx.QueryRow(ctx, `
		UPDATE organization SET logo_object_key = $2, logo_origin = $3
		WHERE id = $1 AND archived_at IS NULL
		RETURNING (SELECT o.logo_object_key FROM organization o WHERE o.id = $1),
		          (SELECT o.logo_origin FROM organization o WHERE o.id = $1)`,
		orgID, *objectKey, *originURL).Scan(&previousKey, &previousOrigin)
	if errors.Is(err, pgx.ErrNoRows) {
		// Archived under this confirmation: nothing to wear a mark, and the
		// same parked object left over.
		return releaseParkedSiteReadLogo(ctx, tx, readID, reclaim)
	}
	if err != nil {
		return nil, fmt.Errorf("bind the website read's logo: %w", err)
	}
	// Handed over, not shared: the record is the object's one reference now.
	// Two rows naming one key would let a later resolve of this organization
	// supersede it, collect the bytes, and leave the dossier pointing at an
	// object nothing can serve. The dossier keeps its reference only while
	// NOTHING else holds it — that is what makes an unadopted mark findable,
	// and the reason this clears only on the run that actually adopted.
	if _, err := tx.Exec(ctx, `
		UPDATE site_read SET logo_object_key = NULL, logo_origin = NULL, updated_at = now()
		WHERE id = $1`, readID); err != nil {
		return nil, fmt.Errorf("hand the website read's logo to the company: %w", err)
	}
	// The site read is what captured this, never the human who confirmed the
	// draft: provenance is written once and never re-derived, and a machine mark
	// recorded under a person's name would make the human-precedence guard
	// refuse every later resolve for a logo nobody chose.
	if err := recordLogoWrite(ctx, tx, orgID,
		resolvedLogoWrite(previousOrigin, *originURL, companySiteReadCapturedBy)); err != nil {
		return nil, err
	}
	// What this confirmation left unreferenced: the mark the record wore before
	// it, if it wore one. On the confirmation that CREATES the company there is
	// none, which is why this stays nil for every onboarding read.
	return supersededObject(previousKey, *objectKey), nil
}

// releaseParkedSiteReadLogo drops the reference a confirmation declined to
// adopt and answers the key it dropped, so the caller collects bytes no record
// wears. The confirmation holds this dossier's row lock, so the drop and the
// answer describe one state.
//
// A caller that owns no object store asks for no release and the reference
// STAYS. The parked key embeds a per-attempt uuid nothing else recorded — it is
// the object's last handle — so clearing it for a caller that cannot delete
// would turn a findable orphan into an unfindable one. It is the confirm path's
// spelling of the guard the worker keeps in front of DiscardSiteReadLogo.
func releaseParkedSiteReadLogo(ctx context.Context, tx pgx.Tx, readID ids.UUID, reclaim bool) (*string, error) {
	// released stays nil for the two outcomes that free nothing: a caller that
	// cannot collect, and bytes a record already wears — the statement matches no
	// row then, and the reference stays with the record.
	var released *string
	if !reclaim {
		return released, nil
	}
	// RETURNING the pre-write key, exactly as the writes above do: the
	// sub-select reads the statement's own snapshot, so it names the object this
	// UPDATE just unreferenced.
	err := tx.QueryRow(ctx, `
		UPDATE site_read sr SET logo_object_key = NULL, logo_origin = NULL, updated_at = now()
		WHERE sr.id = $1 AND sr.logo_object_key IS NOT NULL
		  AND `+logoUnwornByAnyOrganization+`
		RETURNING (SELECT parked.logo_object_key FROM site_read parked WHERE parked.id = $1)`,
		readID).Scan(&released)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("release the website read's unadopted logo: %w", err)
	}
	return released, nil
}

// supersededObject names the object this write orphaned, or nil when it
// orphaned none — the organization had no logo, or the caller re-recorded the
// key already on the row.
func supersededObject(previous *string, objectKey string) *string {
	if previous == nil || *previous == "" || *previous == objectKey {
		return nil
	}
	return previous
}

// LogoHeldByHuman answers whether a person set this organization's logo, for a
// caller that must know BEFORE it does expensive or irreversible work — the
// site read asks first so it neither fetches a logo it may not use nor
// overwrites the object a person's own logo already occupies. It carries the
// record's read gate, so an organization the caller cannot see is not found.
//
// The write applies the same rule again under a row lock: this read is an
// optimization and a byte-safety check, never the authority.
func (s *Store) LogoHeldByHuman(ctx context.Context, id ids.OrganizationID) (bool, error) {
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return false, err
	}
	var held bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "organization", id.UUID); err != nil {
			return err
		}
		var err error
		held, err = logoHeldByHuman(ctx, tx, id)
		return err
	})
	return held, err
}

// logoHeldByHuman reports whether a person's own mark is on this organization
// right now. It reads the same field_provenance layer the provenance display
// reads, so "a human owns this field" has one answer in the product, not two.
//
// What holds a read off is a mark a person chose, not the fact that a person
// once touched the field: someone who REMOVES a logo has asked for the record
// to have none, and leaving their removal standing as a hold would mean the
// company could never be given a face again by any later read. So the row's own
// state is part of the question — an empty field is held by nobody.
func logoHeldByHuman(ctx context.Context, tx pgx.Tx, id ids.OrganizationID) (bool, error) {
	var human bool
	err := tx.QueryRow(ctx, `
		SELECT p.captured_by LIKE 'human:%'
		FROM field_provenance p
		WHERE p.object_type = 'organization' AND p.object_id = $1 AND p.field_name = $2
		  AND EXISTS (SELECT 1 FROM organization o
		               WHERE o.id = $1 AND o.logo_object_key IS NOT NULL)
		ORDER BY p.captured_at DESC, p.id DESC
		LIMIT 1`, id, logoFieldName).Scan(&human)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // no logo provenance yet — nobody holds the field
	}
	if err != nil {
		return false, fmt.Errorf("read organization logo provenance: %w", err)
	}
	return human, nil
}

// OrganizationLogoKey answers where one organization's logo bytes live, for a
// caller that streams them. It returns ErrNotFound both when the organization
// is invisible or absent and when it simply has no logo: to the client those
// are the same answer — draw the monogram — and distinguishing them would leak
// which organizations exist.
func (s *Store) OrganizationLogoKey(ctx context.Context, id ids.OrganizationID) (string, error) {
	// A logo is part of the record, so reading its location is a read of the
	// record and carries the record's gate.
	if err := auth.Require(ctx, "organization", principal.ActionRead); err != nil {
		return "", err
	}
	var key *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "organization", id.UUID); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT logo_object_key FROM organization WHERE id = $1 AND archived_at IS NULL`, id).Scan(&key)
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

// LogoURL renders where a client fetches an organization's logo bytes, or nil
// when the organization has no logo stored. Its query token changes with the
// object key, so replacing a logo cannot leave a browser showing the previous
// cached image at the same URL. The key itself never reaches the wire: it names
// a bucket path, and only a short one-way digest is exposed.
//
// Exported because the account-graph assembly reads organization rows of its
// own and must spell this URL exactly as this module's own reads do — one
// spelling, or a company's face differs between its record and the graph.
func LogoURL(id ids.UUID, objectKey *string) *string {
	if objectKey == nil || *objectKey == "" {
		return nil
	}
	// The prefix versions the representation as well as the object. Version 2
	// removes the transparent square canvas written by older logo uploads, so a
	// browser that cached that letterboxed response must fetch the wide one.
	digest := sha256.Sum256([]byte("logo-display-v2\x00" + *objectKey))
	path := fmt.Sprintf("/v1/organizations/%s/logo?v=%x", id.String(), digest[:6])
	return &path
}

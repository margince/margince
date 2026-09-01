// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// A person choosing their own company's face, and taking it off again.
//
// Both writes go to the anchor organization the workspace names for itself, so
// neither takes a record id: the installation has exactly one company, and an
// id on the wire would be a second answer to a question the workspace already
// answers. The mark they set is a HUMAN write and outranks what a website read
// resolves — the precedence rule the resolve lane reads through
// logoHeldByHuman — until the same person gives the field back.
//
// This store is blob-free, the same division the resolve path keeps: the caller
// stores the bytes first and hands the key here, and takes back the key this
// write superseded so the object nothing references any more can be collected.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetCompanyLogo points the anchor organization at bytes a person uploaded.
// `named` is what the field's history shows for the change — the name of the
// file they chose — and the row's origin stays empty, because an upload was
// resolved from no page.
//
// Unlike the resolve path this never declines: a person replacing their own
// company's mark is the write with the highest standing in this field, so it
// overwrites whatever is there, a mark of their own included.
func (s *Store) SetCompanyLogo(ctx context.Context, objectKey, named string) (supersededKey *string, err error) {
	if err := auth.Require(ctx, "organization", principal.ActionUpdate); err != nil {
		return nil, err
	}
	if objectKey == "" {
		return nil, errors.New("people: an uploaded logo needs the key its bytes are stored at")
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		orgID, err := anchorForLogoWrite(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "organization", orgID.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		var previous, previousOrigin *string
		// The FILE the person chose stands where a resolve puts the page it read:
		// the column records where a mark came from, and leaving it empty for an
		// upload means the NEXT write's before-image says the record wore nothing.
		if err := tx.QueryRow(ctx, `
			UPDATE organization SET logo_object_key = $2, logo_origin = $3
			WHERE id = $1 AND archived_at IS NULL
			RETURNING (SELECT o.logo_object_key FROM organization o WHERE o.id = $1),
			          (SELECT o.logo_origin FROM organization o WHERE o.id = $1)`,
			orgID, objectKey, named).Scan(&previous, &previousOrigin); err != nil {
			return fmt.Errorf("set the company logo: %w", err)
		}
		supersededKey = supersededObject(previous, objectKey)
		return recordLogoWrite(ctx, tx, orgID, logoWrite{
			previousOrigin: previousOrigin, origin: &named,
			source: companySourceHuman, by: by,
		})
	})
	if err != nil {
		return nil, err
	}
	return supersededKey, nil
}

// ClearCompanyLogo takes the mark off the anchor organization and hands back
// the key its bytes lived at, so the caller can collect them. A company that
// had no mark is not an error: nothing is written, nothing is collected, and
// the caller's request is already true.
func (s *Store) ClearCompanyLogo(ctx context.Context) (supersededKey *string, err error) {
	if err := auth.Require(ctx, "organization", principal.ActionUpdate); err != nil {
		return nil, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		orgID, err := anchorForLogoWrite(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "organization", orgID.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		var previous, previousOrigin *string
		if err := tx.QueryRow(ctx, `
			UPDATE organization SET logo_object_key = NULL, logo_origin = NULL
			WHERE id = $1 AND archived_at IS NULL
			RETURNING (SELECT o.logo_object_key FROM organization o WHERE o.id = $1),
			          (SELECT o.logo_origin FROM organization o WHERE o.id = $1)`,
			orgID).Scan(&previous, &previousOrigin); err != nil {
			return fmt.Errorf("clear the company logo: %w", err)
		}
		if previous == nil || *previous == "" {
			return nil
		}
		supersededKey = previous
		return recordLogoWrite(ctx, tx, orgID, logoWrite{
			previousOrigin: previousOrigin, source: companySourceHuman, by: by,
		})
	})
	if err != nil {
		return nil, err
	}
	return supersededKey, nil
}

// anchorForLogoWrite names the installation's own company and checks the caller
// may write it. Each writer takes the row lock itself, beside its own statement:
// the lock is what makes the pair of statements that follows ONE decision —
// without it an upload and a website read landing together would each read a
// field the other was in the middle of writing, and the row would end up naming
// one attempt's key with the other's provenance. Live rows only, matching every
// other mutation here: an archived company takes no new mark.
func anchorForLogoWrite(ctx context.Context, tx pgx.Tx) (ids.OrganizationID, error) {
	orgID, err := anchorOrganization(ctx, tx, false)
	if err != nil {
		return ids.OrganizationID{}, err
	}
	if err := auth.EnsureWritable(ctx, tx, "organization", orgID.UUID); err != nil {
		return ids.OrganizationID{}, err
	}
	return orgID, nil
}

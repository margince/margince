// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Acquisition sources are the administered business channels a deal can be
// attributed to (deal_acquisition_source): referral, inbound, partner, event.
//
// DELIBERATELY NOT the lead-source vocabulary, though the two look alike.
// `lead_source` answers how a RECORD reached Margince — it carries connector
// families (`connector:apollo:…`), import and crawl values, and a scoring
// intent the lead scorer reads. This answers how an OPPORTUNITY reached the
// business, which is a different question with different values: a deal typed
// in by hand can be a referral, and an imported one can be outbound. Sharing
// one table would force every reader to know which of the two questions a row
// was answering, and the scorer would start weighting deal channels it was
// never calibrated on.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// acquisitionVocabularyObject is the RBAC object this catalog is gated by:
// the custom-field catalog's posture — everyone reads, admin/ops write —
// which is exactly this list's posture, and lead_source is gated the same
// way for the same reason. Adding a policy-matrix object for a list that
// behaves identically to one already there would be a second answer to a
// settled question.
const acquisitionVocabularyObject = "custom_field"

// dealAcquisitionSourceConstraint is the FK a deal's acquisition_source
// violates when it names a key the catalog does not hold. The create and
// update paths both map it to the same field error.
const dealAcquisitionSourceConstraint = "deal_acquisition_source_fkey"

// acquisitionSourceColumns is the catalog's wire projection.
const acquisitionSourceColumns = `id, key, label, sort_order, active, system, version, created_at, updated_at`

// UnknownAcquisitionSourceError refuses a key the catalog does not hold.
type UnknownAcquisitionSourceError struct{}

func (e *UnknownAcquisitionSourceError) Error() string {
	return "acquisition_source names no source in the catalog"
}

// FieldFault names the caller's own field, so the fix is to correct the
// source rather than to hunt for a missing record.
func (e *UnknownAcquisitionSourceError) FieldFault() (field, code, message string) {
	return "acquisition_source", "unknown_acquisition_source", e.Error()
}

// RetiredAcquisitionSourceError refuses a retired key for a NEW assignment.
// A deal already carrying one keeps it: the refusal is about choosing the
// value now, not about the value having ever been chosen.
type RetiredAcquisitionSourceError struct{ Key string }

func (e *RetiredAcquisitionSourceError) Error() string {
	return "acquisition source " + e.Key + " is retired and cannot be newly assigned"
}

func (e *RetiredAcquisitionSourceError) FieldFault() (field, code, message string) {
	return "acquisition_source", "acquisition_source_retired", e.Error()
}

// CreateAcquisitionSourceInput adds one channel to the catalog.
type CreateAcquisitionSourceInput struct {
	Key       string
	Label     string
	SortOrder int
}

// UpdateAcquisitionSourceInput relabels, reorders or retires one. The key is
// absent on purpose: it is the value deals carry, and changing it would
// rewrite what every deal holding it means.
type UpdateAcquisitionSourceInput struct {
	Label     *string
	SortOrder *int
	Active    *bool
	IfVersion *int64
}

// nonSlugRun matches every run of characters a key may not contain, so a
// label becomes a key by collapsing them to single underscores.
var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

// deriveAcquisitionKey slugs a label into a key. Mirrors the lead
// vocabulary's rule rather than inventing a second spelling of "slug".
func deriveAcquisitionKey(label string) string {
	key := nonSlugRun.ReplaceAllString(strings.ToLower(strings.TrimSpace(label)), "_")
	return strings.Trim(key, "_")
}

// ListAcquisitionSources answers the whole catalog, retired entries included:
// a deal carrying a retired key still has to render its label, and a filter
// still has to find those deals.
func (s *Store) ListAcquisitionSources(ctx context.Context) ([]crmcontracts.AcquisitionSource, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.AcquisitionSource
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+acquisitionSourceColumns+` FROM deal_acquisition_source ORDER BY sort_order, label`)
		if err != nil {
			return fmt.Errorf("list acquisition sources: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			source, err := scanAcquisitionSource(rows)
			if err != nil {
				return err
			}
			out = append(out, source)
		}
		return rows.Err()
	})
	return out, err
}

// CreateAcquisitionSource adds a channel.
func (s *Store) CreateAcquisitionSource(
	ctx context.Context, in CreateAcquisitionSourceInput,
) (crmcontracts.AcquisitionSource, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionCreate); err != nil {
		return crmcontracts.AcquisitionSource{}, err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return crmcontracts.AcquisitionSource{}, &values.ParseError{
			Field: "label", Code: "required", Message: "label is required"}
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = deriveAcquisitionKey(label)
	}
	if key == "" || key != strings.ToLower(key) {
		return crmcontracts.AcquisitionSource{}, &values.ParseError{
			Field: "key", Code: "invalid_key", Message: "key must be a non-empty lowercase value"}
	}
	var out crmcontracts.AcquisitionSource
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		id := ids.NewV7()
		_, err := tx.Exec(ctx,
			`INSERT INTO deal_acquisition_source (id, key, label, sort_order) VALUES ($1, $2, $3, $4)`,
			id, key, label, in.SortOrder)
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert acquisition source: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", "deal_acquisition_source", id, nil,
			map[string]any{"key": key, "label": label}); err != nil {
			return err
		}
		out, err = readAcquisitionSource(ctx, tx, id)
		return err
	})
	return out, err
}

// UpdateAcquisitionSource relabels, reorders or retires one.
//
// Retirement is `active: false` and there is no delete: a key a deal has ever
// carried must stay resolvable, or that deal's own history stops rendering.
func (s *Store) UpdateAcquisitionSource(
	ctx context.Context, id ids.UUID, in UpdateAcquisitionSourceInput,
) (crmcontracts.AcquisitionSource, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.AcquisitionSource{}, err
	}
	if in.Label != nil && strings.TrimSpace(*in.Label) == "" {
		return crmcontracts.AcquisitionSource{}, &values.ParseError{
			Field: "label", Code: "required", Message: "label cannot be blank"}
	}
	var out crmcontracts.AcquisitionSource
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		before, err := readAcquisitionSourceForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		patch := storekit.NewPatch()
		if in.Label != nil {
			patch.Set("label", before.Label, strings.TrimSpace(*in.Label))
		}
		if in.SortOrder != nil {
			patch.Set("sort_order", before.SortOrder, *in.SortOrder)
		}
		if in.Active != nil {
			patch.Set("active", before.Active, *in.Active)
		}
		if patch.Empty() {
			out, err = readAcquisitionSource(ctx, tx, id)
			return err
		}
		if in.IfVersion != nil && *in.IfVersion != before.Version {
			return apperrors.ErrConflict
		}
		if err := patch.ApplyWithVersion(ctx, tx, "deal_acquisition_source", id, before.Version); err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "deal_acquisition_source", id,
			patch.Before(), patch.After()); err != nil {
			return err
		}
		out, err = readAcquisitionSource(ctx, tx, id)
		return err
	})
	return out, err
}

// ensureAssignableAcquisitionSource refuses a retired key for a new
// assignment while admitting the one a deal already holds.
//
// The row is locked, not merely read: a retirement committing between this
// check and the deal write would otherwise let a key through that the
// administrator had just withdrawn — the check would have been true when it
// ran and false by the time it mattered.
func ensureAssignableAcquisitionSource(ctx context.Context, tx pgx.Tx, key string, current *string) error {
	if current != nil && *current == key {
		return nil
	}
	var active bool
	err := tx.QueryRow(ctx,
		`SELECT active FROM deal_acquisition_source WHERE key = $1 FOR UPDATE`, key).Scan(&active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &UnknownAcquisitionSourceError{}
		}
		return fmt.Errorf("read acquisition source: %w", err)
	}
	if !active {
		return &RetiredAcquisitionSourceError{Key: key}
	}
	return nil
}

func readAcquisitionSource(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.AcquisitionSource, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+acquisitionSourceColumns+` FROM deal_acquisition_source WHERE id = $1`, id)
	return scanAcquisitionSource(row)
}

// acquisitionSourceRow is the locked read an update patches against.
type acquisitionSourceRow struct {
	Label     string
	SortOrder int
	Active    bool
	Version   int64
}

func readAcquisitionSourceForUpdate(ctx context.Context, tx pgx.Tx, id ids.UUID) (acquisitionSourceRow, error) {
	var out acquisitionSourceRow
	err := tx.QueryRow(ctx,
		`SELECT label, sort_order, active, version FROM deal_acquisition_source WHERE id = $1 FOR UPDATE`, id).
		Scan(&out.Label, &out.SortOrder, &out.Active, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("read acquisition source: %w", err)
	}
	return out, nil
}

func scanAcquisitionSource(row pgx.Row) (crmcontracts.AcquisitionSource, error) {
	var out crmcontracts.AcquisitionSource
	var id ids.UUID
	var version int64
	if err := row.Scan(&id, &out.Key, &out.Label, &out.SortOrder, &out.Active,
		&out.System, &version, &out.CreatedAt, &out.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, apperrors.ErrNotFound
		}
		return out, fmt.Errorf("scan acquisition source: %w", err)
	}
	out.Id = openapi_types.UUID(id)
	out.Version = &version
	return out, nil
}

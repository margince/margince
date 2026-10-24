// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Lead origins answer "where did this lead come from" (lead_origin): referral,
// event, a cold list. A human, an importer or capture sets one. It is the only
// lead fact the score, the origin filter and the report read.
//
// It is not lead.source, which says how the row reached Margince and never
// scores. One column for both questions made an import look like a low-intent
// enquiry.

import (
	"context"
	"errors"
	"fmt"
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

const (
	leadOriginField       = "origin"
	leadOriginDetailField = "origin_detail"
	leadOriginLabelField  = "label"
	leadOriginKeyField    = "key"
	leadOriginIntentField = "intent"
	msgLabelNotEmpty      = "label must not be empty"
	msgLabelRequired      = "label is required"
	// LeadOriginDetailMax is the longest origin_detail, in characters.
	LeadOriginDetailMax = values.MaxLeadOriginDetailRunes
	// leadOriginFilterUnset is the list filter's word for a lead with no origin.
	leadOriginFilterUnset = "unset"
)

// OriginIntents maps an origin key to the intent the scorer weighs.
type OriginIntents map[string]SourceIntent

// Of answers the intent for one lead's origin. A lead with no origin, and one
// whose key the table does not carry, scores neutral.
func (m OriginIntents) Of(origin *string) SourceIntent {
	if origin == nil {
		return SourceIntentNeutral
	}
	if intent, ok := m[*origin]; ok {
		return intent
	}
	return SourceIntentNeutral
}

// LoadOriginIntents reads the installation's weighting inside the caller's
// transaction. A retired origin still weighs: a lead keeps its value when the
// origin is retired, and so keeps its score.
func LoadOriginIntents(ctx context.Context, tx pgx.Tx) (OriginIntents, error) {
	rows, err := tx.Query(ctx, `SELECT key, intent FROM lead_origin`)
	if err != nil {
		return nil, fmt.Errorf("load lead origin intents: %w", err)
	}
	defer rows.Close()
	out := OriginIntents{}
	for rows.Next() {
		var key, intent string
		if err := rows.Scan(&key, &intent); err != nil {
			return nil, err
		}
		out[key] = SourceIntent(intent)
	}
	return out, rows.Err()
}

// ResolveOrigin maps the free text a source system used onto an active catalog
// key. The text may be the key itself, or it may derive to the same key as
// one label. No match, or two labels that derive alike, answers false
// and never a guessed key. The importer then keeps the wording instead of
// misfiling the lead.
func ResolveOrigin(ctx context.Context, tx pgx.Tx, text string) (string, bool, error) {
	exact := strings.TrimSpace(text)
	wanted := deriveSourceKey(text)
	if wanted == "" {
		return "", false, nil
	}
	rows, err := tx.Query(ctx, `SELECT key, label FROM lead_origin WHERE active ORDER BY sort_order, key`)
	if err != nil {
		return "", false, fmt.Errorf("resolve lead origin: %w", err)
	}
	defer rows.Close()
	var labelMatches []string
	for rows.Next() {
		var key, label string
		if err := rows.Scan(&key, &label); err != nil {
			return "", false, err
		}
		if key == exact {
			return key, true, nil
		}
		if deriveSourceKey(label) == wanted {
			labelMatches = append(labelMatches, key)
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	if len(labelMatches) != 1 {
		return "", false, nil
	}
	return labelMatches[0], true, nil
}

// UnknownLeadOriginError refuses a key the catalog does not hold.
type UnknownLeadOriginError struct{ Key string }

func (e *UnknownLeadOriginError) Error() string {
	return "origin " + e.Key + " is not in the lead origin list; pick one from Settings › Data model"
}

// FieldFault names the origin as the invalid input.
func (e *UnknownLeadOriginError) FieldFault() (field, code, message string) {
	return leadOriginField, "unknown_lead_origin", e.Error()
}

// InactiveLeadOriginError refuses a retired origin for a new assignment. A
// lead already carrying one keeps it.
type InactiveLeadOriginError struct{ Key string }

func (e *InactiveLeadOriginError) Error() string {
	return "origin " + e.Key + " is retired; pick an active origin or reactivate it in Settings › Data model"
}

// FieldFault names the origin as the invalid input.
func (e *InactiveLeadOriginError) FieldFault() (field, code, message string) {
	return leadOriginField, "inactive_lead_origin", e.Error()
}

// ensureAssignableOrigin refuses an unknown or retired key for a new
// assignment and admits the one the lead already holds. The row is locked, so
// a retirement committing between this check and the lead write cannot let a
// withdrawn key through.
func ensureAssignableOrigin(ctx context.Context, tx pgx.Tx, key string, current *string) error {
	if current != nil && *current == key {
		return nil
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT active FROM lead_origin WHERE key = $1 FOR SHARE`, key).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return &UnknownLeadOriginError{Key: key}
	}
	if err != nil {
		return fmt.Errorf("check lead origin %q: %w", key, err)
	}
	if !active {
		return &InactiveLeadOriginError{Key: key}
	}
	return nil
}

// CreateLeadOriginInput is one new administered origin.
type CreateLeadOriginInput struct {
	Key       string // optional; derived from Label when empty
	Label     string
	Intent    SourceIntent
	SortOrder int
}

// UpdateLeadOriginInput is a sparse patch; nil leaves the column alone.
type UpdateLeadOriginInput struct {
	Label     *string
	Intent    *SourceIntent
	SortOrder *int
	Active    *bool
}

// leadOriginColumns renders the select list. lead_count carries the caller's
// row scope: a narrowed actor counts only the leads they may see.
func leadOriginColumns(ctx context.Context, args *[]any) (string, error) {
	arg := func(v any) int { *args = append(*args, v); return len(*args) }
	scope, err := visibleLeadScope(ctx, arg)
	if err != nil {
		return "", err
	}
	return `id, key, label, intent, sort_order, active, system, version, created_at, updated_at,
	(SELECT count(*) FROM lead WHERE archived_at IS NULL AND ` + scope + ` AND origin = lead_origin.key)`, nil
}

func scanLeadOrigin(row pgx.Row) (crmcontracts.LeadOrigin, error) {
	var out crmcontracts.LeadOrigin
	var id ids.UUID
	var intent string
	var system bool
	var version int64
	var count int
	if err := row.Scan(&id, &out.Key, &out.Label, &intent, &out.SortOrder, &out.Active, &system, &version,
		&out.CreatedAt, &out.UpdatedAt, &count); err != nil {
		return out, err
	}
	out.Id = openapi_types.UUID(id)
	out.Intent = crmcontracts.LeadSourceIntent(intent)
	out.System = &system
	out.Version = &version
	out.LeadCount = &count
	return out, nil
}

// readLeadOrigin reads one row; lock takes it FOR UPDATE first so a patch
// built from the read cannot be overtaken by a concurrent writer.
func readLeadOrigin(ctx context.Context, tx pgx.Tx, id ids.UUID, lock bool) (crmcontracts.LeadOrigin, error) {
	if lock {
		if _, err := storekit.LockRow(ctx, tx, "lead_origin", id, storekit.NoArchiveColumn); err != nil {
			return crmcontracts.LeadOrigin{}, err
		}
	}
	args := []any{id}
	cols, err := leadOriginColumns(ctx, &args)
	if err != nil {
		return crmcontracts.LeadOrigin{}, err
	}
	out, err := scanLeadOrigin(tx.QueryRow(ctx, `SELECT `+cols+` FROM lead_origin WHERE id = $1`, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	return out, err
}

// ListLeadOrigins answers the whole catalog, retired entries included.
func (s *Store) ListLeadOrigins(ctx context.Context) (crmcontracts.LeadOriginListResponse, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionRead); err != nil {
		return crmcontracts.LeadOriginListResponse{}, err
	}
	out := crmcontracts.LeadOriginListResponse{Data: []crmcontracts.LeadOrigin{}}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		cols, err := leadOriginColumns(ctx, &args)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM lead_origin ORDER BY sort_order, label, id`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			origin, err := scanLeadOrigin(rows)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, origin)
		}
		return rows.Err()
	})
	return out, err
}

// CreateLeadOrigin adds an origin. A key already in the list is a 409; a blank
// label, a bad key or an unknown intent is a 422.
func (s *Store) CreateLeadOrigin(ctx context.Context, in CreateLeadOriginInput) (crmcontracts.LeadOrigin, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionCreate); err != nil {
		return crmcontracts.LeadOrigin{}, err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return crmcontracts.LeadOrigin{}, &values.ParseError{Field: leadOriginLabelField, Code: codeRequired, Message: msgLabelRequired}
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = deriveSourceKey(label)
	}
	if key == "" || key != strings.ToLower(key) {
		return crmcontracts.LeadOrigin{}, &values.ParseError{Field: leadOriginKeyField, Code: "invalid_key", Message: "key must be a non-empty lowercase value"}
	}
	if key == leadOriginFilterUnset {
		return crmcontracts.LeadOrigin{}, &values.ParseError{
			Field: leadOriginKeyField, Code: "reserved_key",
			Message: "unset is reserved: the lead list uses it to mean no origin recorded",
		}
	}
	intent := in.Intent
	if intent == "" {
		intent = SourceIntentNeutral
	}
	if _, err := ParseSourceIntent(string(intent)); err != nil {
		return crmcontracts.LeadOrigin{}, err
	}
	var out crmcontracts.LeadOrigin
	err := s.tx(ctx, func(tx pgx.Tx) error {
		id := ids.NewV7()
		_, err := tx.Exec(ctx,
			`INSERT INTO lead_origin (id, key, label, intent, sort_order) VALUES ($1, $2, $3, $4, $5)`,
			id, key, label, string(intent), in.SortOrder)
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert lead origin: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", "lead_origin", id, nil,
			map[string]any{leadOriginKeyField: key, leadOriginLabelField: label, leadOriginIntentField: string(intent)}); err != nil {
			return err
		}
		out, err = readLeadOrigin(ctx, tx, id, false)
		return err
	})
	return out, err
}

// UpdateLeadOrigin renames, reorders, re-weights or retires an origin. The key
// never changes: it is the value leads carry.
func (s *Store) UpdateLeadOrigin(ctx context.Context, id ids.UUID, in UpdateLeadOriginInput) (crmcontracts.LeadOrigin, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.LeadOrigin{}, err
	}
	if in.Label != nil && strings.TrimSpace(*in.Label) == "" {
		return crmcontracts.LeadOrigin{}, &values.ParseError{Field: leadOriginLabelField, Code: codeRequired, Message: msgLabelNotEmpty}
	}
	if in.Intent != nil {
		if _, err := ParseSourceIntent(string(*in.Intent)); err != nil {
			return crmcontracts.LeadOrigin{}, err
		}
	}
	var out crmcontracts.LeadOrigin
	err := s.tx(ctx, func(tx pgx.Tx) error {
		before, err := readLeadOrigin(ctx, tx, id, true)
		if err != nil {
			return err
		}
		p := storekit.NewPatch()
		if in.Label != nil {
			p.Set("label", before.Label, strings.TrimSpace(*in.Label))
		}
		if in.Intent != nil {
			p.Set("intent", string(before.Intent), string(*in.Intent))
		}
		if in.SortOrder != nil {
			p.Set("sort_order", before.SortOrder, *in.SortOrder)
		}
		if in.Active != nil {
			p.Set("active", before.Active, *in.Active)
		}
		if p.Empty() {
			out = before
			return nil
		}
		// The row lock readLeadOrigin took is the whole of the serialization:
		// the table has no archived_at for ApplyWithVersion to narrow by.
		lock, err := storekit.LockRow(ctx, tx, "lead_origin", id, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "lead_origin", id, p.Before(), p.After()); err != nil {
			return err
		}
		out, err = readLeadOrigin(ctx, tx, id, false)
		return err
	})
	return out, err
}

// DeleteLeadOrigin removes an origin nothing depends on. A built-in origin, or
// one any lead carries (archived leads included), is a 409: retire it instead.
func (s *Store) DeleteLeadOrigin(ctx context.Context, id ids.UUID) error {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		current, err := readLeadOrigin(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if current.System != nil && *current.System {
			return apperrors.ErrConflict
		}
		var inUse bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM lead WHERE origin = $1)`, current.Key).Scan(&inUse); err != nil {
			return fmt.Errorf("check lead origin use: %w", err)
		}
		if inUse {
			return apperrors.ErrConflict
		}
		if _, err := tx.Exec(ctx, `DELETE FROM lead_origin WHERE id = $1`, id); err != nil {
			return fmt.Errorf("delete lead origin: %w", err)
		}
		_, err = storekit.Audit(ctx, tx, "erase", "lead_origin", id,
			map[string]any{leadOriginKeyField: current.Key, leadOriginLabelField: current.Label}, nil)
		return err
	})
}

// leadOriginLabelSQL is the origin in the words the catalog gives it, which is
// what the Origin column prints and what the sort arranges by.
const leadOriginLabelSQL = `(SELECT o.label FROM lead_origin o WHERE o.key = lead.origin)`

// leadOriginClause renders the list's origin filter: one exact key bound as a
// parameter, never a pattern, so `%`, `_` and `\` name themselves. `unset`
// names the leads with no origin; createLeadOrigin refuses that key.
func leadOriginClause(origin string, arg func(any) int) string {
	if origin == leadOriginFilterUnset {
		return "lead.origin IS NULL"
	}
	return storekit.SQLf("lead.origin = $%d", arg(origin))
}

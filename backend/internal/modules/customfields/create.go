// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// pgDuplicateColumn is SQLSTATE 42701: the ALTER TABLE's ADD COLUMN hit
// a column that already exists. Under the advisory lock the pre-check
// answers this case first; the SQLSTATE branch is the belt for a column
// added outside the engine (a fork migration racing a create).
const pgDuplicateColumn = "42701"

// lockTimedOut reports whether err is the bounded lock wait firing — the
// retryable ErrTableBusy answer, never a 500. The SQLSTATE itself is storekit's
// to name, so this module reads the classification rather than the code.
func lockTimedOut(err error) bool {
	return storekit.IsLockTimeout(err)
}

// Create is the single chokepoint allowed to run a runtime ALTER TABLE
// (custom-fields.md "one chokepoint"): it validates spec, refuses a
// structural request, derives slug/column_name, and then commits the
// physical column + the catalog row + one audit entry in ONE transaction
// on the owner-privileged schema pool — Postgres transactional DDL makes
// the three land or roll back together.
//
// This does not use database.WithTx. That helper runs on the app pool, whose
// margince_app role carries DML-only grants and cannot ALTER a core table.
// The whole transaction runs as the owner role and never uses SET ROLE. The
// owner is not a member of margince_app on a real install.
func (s *Service) Create(ctx context.Context, spec FieldSpec) (crmcontracts.CustomField, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionCreate); err != nil {
		return crmcontracts.CustomField{}, err
	}
	if errs := Validate(spec); len(errs) > 0 {
		return crmcontracts.CustomField{}, &ValidationError{Errors: errs}
	}
	if err := requireLabelWithin(spec.Label); err != nil {
		return crmcontracts.CustomField{}, err
	}
	if IsStructural(spec.Label) {
		return crmcontracts.CustomField{}, ErrStructural
	}
	slug := DeriveSlug(spec.Label)
	if s.schemaPool == nil {
		return crmcontracts.CustomField{}, ErrSchemaChangesUnavailable
	}
	creator, err := createdBy(ctx)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}

	// pool-exempt: DDL runs as the schema owner, so the owner pool opens the transaction; the catalog row and its audit row commit in it.
	tx, err := s.schemaPool.Begin(ctx)
	if err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: begin schema tx: %w", err)
	}
	// Error-path safety net only. After Commit this is pgx's ErrTxClosed
	// no-op, and on the error path the caller must see the operation's own
	// error (the WithTx spelling).
	//craft:ignore swallowed-errors deferred rollback of a committed tx is a designed no-op; real failures already left through the operation error
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := s.createInTx(ctx, tx, spec, creator, slug)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: commit schema tx: %w", err)
	}
	return out, nil
}

// createInTx is Create's transaction body: add the column, then write the
// catalog row and audit entry, all as the schema pool's owner role.
func (s *Service) createInTx(ctx context.Context, tx pgx.Tx, spec FieldSpec, creator ids.UUID, slug string) (crmcontracts.CustomField, error) {
	spec.PipelineIDs = storedPipelines(spec.PipelineIDs)
	if err := livePipelines(ctx, tx, spec.PipelineIDs); err != nil {
		return crmcontracts.CustomField{}, err
	}
	column, err := addColumn(ctx, tx, spec, slug)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	var currency *string
	if spec.Type == TypeCurrency {
		currency = spec.Currency
	}
	optionsArg, err := optionsJSON(spec)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	out, err := insertCatalogRow(ctx, tx, []catalogBinding{
		{fieldObject, spec.Object},
		{fieldSlug, slug},
		{fieldLabel, spec.Label},
		{fieldType, spec.Type},
		{fieldColumn, column},
		{fieldCurrency, currency},
		{fieldOptions, optionsArg},
		{fieldShow, spec.ShowInCreate == nil || *spec.ShowInCreate},
		{fieldKinds, storedKinds(spec.ActivityKinds)},
		{fieldPipelines, spec.PipelineIDs},
		{"created_by", creator},
	})
	if storekit.IsUniqueViolation(err) {
		// The (object, slug/column_name) unique index fired: a duplicate
		// that raced past the pre-check. The failed INSERT rolls the whole
		// transaction back, ALTER included.
		return crmcontracts.CustomField{}, &LabelTakenError{message: fmt.Sprintf(
			"a custom field named %q already exists on %s", slug, spec.Object)}
	}
	if err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: inserting catalog row: %w", err)
	}

	if err := auditCreate(ctx, tx, out, column, spec.Source); err != nil {
		return crmcontracts.CustomField{}, err
	}
	return out, nil
}

// auditCreate writes the create's audit entry.
func auditCreate(ctx context.Context, tx pgx.Tx, out crmcontracts.CustomField, column, source string) error {
	after := map[string]any{
		fieldObject: string(out.Object), fieldSlug: out.Slug, fieldLabel: out.Label,
		fieldType: string(out.Type), fieldColumn: column, fieldStatus: string(out.Status),
		fieldShow: out.ShowInCreate,
	}
	if out.Currency != nil {
		after[fieldCurrency] = *out.Currency
	}
	if out.Options != nil {
		after[fieldOptions] = *out.Options
	}
	if out.ActivityKinds != nil {
		after[fieldKinds] = *out.ActivityKinds
	}
	if out.PipelineIds != nil {
		after[fieldPipelines] = *out.PipelineIds
	}
	// Audit-only by ratification: the closed event catalog (events.md §5)
	// defines no custom_field.* type — the spec's custom-fields.md §Events
	// pins the audit entry as the add/rename/retire trail, and a
	// cross-object catalog change has no single family stream to ride
	// (the attachments precedent in writeshape_test.go).
	if _, err := storekit.AuditWithEvidence(ctx, tx, "create", rbacObject, ids.UUID(out.Id),
		nil, after, map[string]any{fieldSource: source}); err != nil {
		return fmt.Errorf("customfields: audit write: %w", err)
	}
	return nil
}

// addColumn is the schema half of a create: lock → collision pre-check → column
// allocation → ALTER. The column is allocated under the lock so two creates of
// one name cannot pick the same one. It answers the column it added.
func addColumn(ctx context.Context, tx pgx.Tx, spec FieldSpec, slug string) (string, error) {
	if err := serializeSchemaChange(ctx, tx, spec.Object); err != nil {
		return "", err
	}
	if err := refuseTakenLabel(ctx, tx, spec.Object, ids.Nil, spec.Label); err != nil {
		return "", err
	}
	column, err := allocateColumn(ctx, tx, spec.Object, slug)
	if err != nil {
		return "", err
	}
	if err := refuseTakenColumn(ctx, tx, spec.Object, column); err != nil {
		return "", err
	}
	ddl, err := BuildDDL(spec.Object, column, spec)
	if err != nil {
		return "", err
	}

	// The one statement that needs the owner role. 42701 can still fire
	// despite the pre-check when the column arrived outside the engine's
	// serialization (a fork migration), and answers the same conflict.
	// 55P03 is the bounded lock wait firing: the table is busy, the caller
	// retries.
	if _, err := tx.Exec(ctx, ddl); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgDuplicateColumn {
			return "", &ColumnTakenError{Column: column}
		}
		if lockTimedOut(err) {
			return "", ErrTableBusy
		}
		return "", fmt.Errorf("customfields: adding column: %w", err)
	}

	return column, nil
}

// catalogBinding is one column of the catalog INSERT and its value. The column
// list, the placeholders and the arguments all derive from one list of them.
type catalogBinding struct {
	column string
	value  any
}

// insertCatalogRow writes one custom_field row and reads it back in the
// catalogColumns shape. Every column is a compile-time literal of this package.
func insertCatalogRow(ctx context.Context, tx pgx.Tx, bindings []catalogBinding) (crmcontracts.CustomField, error) {
	columns := make([]string, len(bindings))
	args := make([]any, len(bindings))
	for i, b := range bindings {
		columns[i], args[i] = b.column, b.value
	}
	out, err := scanCustomField(tx.QueryRow(ctx, storekit.SQLf(`INSERT INTO custom_field (%s) VALUES (%s) RETURNING %s`,
		strings.Join(columns, ", "), storekit.Placeholders(args), catalogColumns), args...))
	if err == nil {
		forgetActiveColumns(tx, string(out.Object))
	}
	return out, err
}

// serializeSchemaChange is the shared arming step of both DDL paths
// (createInTx and setOptionsInTx):
//
// SET LOCAL lock_timeout bounds every lock wait in this transaction —
// the ALTER TABLE ahead needs ACCESS EXCLUSIVE, and unbounded, a single
// long-running reader queues that request while EVERY subsequent DML on
// the shared core table queues behind it: a platform-wide stall from one
// admin call. Timing out (SQLSTATE 55P03) answers the retryable
// ErrTableBusy instead; transaction-local, so the pooled connection
// reverts at COMMIT/ROLLBACK.
//
// The transaction-scoped advisory lock, keyed on the target table,
// removes the 42701 race window between concurrent ADD COLUMNs on the
// same shared table: whoever holds it sees the loser's committed column
// in the pre-check instead of colliding mid-ALTER; auto-released at
// COMMIT/ROLLBACK. lock_timeout bounds this wait too — a peer stuck
// mid-DDL answers the same busy signal.
func serializeSchemaChange(ctx context.Context, tx pgx.Tx, object string) error {
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
		return fmt.Errorf("customfields: bounding lock waits: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('customfields:' || $1, 0))`, object); err != nil {
		if lockTimedOut(err) {
			return ErrTableBusy
		}
		return fmt.Errorf("customfields: serializing schema change: %w", err)
	}
	return nil
}

// refuseTakenColumn is the collision answer, decided under the advisory
// lock BEFORE the ALTER: the catalog's uniques govern catalog rows, and
// they cannot see a physical column that no catalog row claims. A live
// column WITH a catalog row is the ordinary duplicate-slug conflict; a
// live column without one is a name already taken on the table — a
// half-applied change, or a core column the cf_ namespace has run into.
// Either way an honest 409 naming the remedy, rather than an ALTER that
// fails mid-transaction.
func refuseTakenColumn(ctx context.Context, tx pgx.Tx, object, column string) error {
	var columnExists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		  WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)`,
		object, column).Scan(&columnExists); err != nil {
		return fmt.Errorf("customfields: probing column namespace: %w", err)
	}
	if !columnExists {
		return nil
	}
	var claimed bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM custom_field
		  WHERE object = $1 AND column_name = $2)`,
		object, column).Scan(&claimed); err != nil {
		return fmt.Errorf("customfields: probing catalog claim: %w", err)
	}
	if claimed {
		return &LabelTakenError{message: fmt.Sprintf("a custom field with column %q already exists on %s", column, object)}
	}
	return &ColumnTakenError{Column: column}
}

// maxColumnSuffix bounds allocateColumn so the longest generated identifier,
// cf_<slug>_<n>_text_max_20000, stays under Postgres's 63-byte cap.
const maxColumnSuffix = 999

// allocateColumn picks the physical column for a new field of slug: cf_<slug>,
// else the first cf_<slug>_<n> no catalog row of the object owns. A retired field
// keeps its column, so a new field with its slug gets a different one. The pick
// is deterministic and, under the schema lock, race-free.
func allocateColumn(ctx context.Context, tx pgx.Tx, object, slug string) (string, error) {
	base := ColumnName(slug)
	rows, err := tx.Query(ctx,
		`SELECT column_name FROM custom_field WHERE object = $1 AND starts_with(column_name, $2)`, object, base)
	if err != nil {
		return "", fmt.Errorf("customfields: reading claimed columns: %w", err)
	}
	owned, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("customfields: reading claimed columns: %w", err)
	}
	claimed := make(map[string]bool, len(owned))
	for _, name := range owned {
		claimed[name] = true
	}
	for n := 1; n <= maxColumnSuffix; n++ {
		candidate := base
		if n > 1 {
			candidate = fmt.Sprintf("%s_%d", base, n)
		}
		if !claimed[candidate] {
			return candidate, nil
		}
	}
	return "", &LabelTakenError{message: fmt.Sprintf(
		"too many custom fields on %s were named %q; choose a different label", object, slug)}
}

// requireLabelWithin refuses a label past the label limit, measured as it is
// stored: the label keeps the spaces it was sent with.
func requireLabelWithin(label string) error {
	return httperr.RequireWithin(fieldLabel, label, values.TextLabelCharacters)
}

// foldedLabel is a stored label as the collision test reads it: trimmed of any
// whitespace, as strings.TrimSpace trims the label being written, and lower-cased.
const foldedLabel = `lower(regexp_replace(label, '^\s+|\s+$', '', 'g'))`

// refuseTakenLabel keeps one label from naming two live fields of an object. A
// label whose text another field holds reads the same in a picker and a filter.
// A label whose slug another field holds would collide on the unique key.
// A retired field holds neither, because every screen marks it retired.
// Create and rename both ask it under the schema advisory lock. A create
// passes ids.Nil, which no row has.
func refuseTakenLabel(ctx context.Context, tx pgx.Tx, object string, id ids.UUID, label string) error {
	var heldLabel, heldSlug string
	err := tx.QueryRow(ctx,
		`SELECT label, slug FROM custom_field
		  WHERE object = $1 AND id <> $2 AND status <> $5 AND (slug = $3 OR `+foldedLabel+` = $4)
		  ORDER BY (`+foldedLabel+` = $4) DESC LIMIT 1`,
		object, id, DeriveSlug(label), strings.ToLower(strings.TrimSpace(label)), statusRetired).Scan(&heldLabel, &heldSlug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("customfields: probing label namespace: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(heldLabel), strings.TrimSpace(label)) {
		return &LabelTakenError{message: fmt.Sprintf("a custom field labelled %q already exists on %s", label, object)}
	}
	return &LabelTakenError{message: fmt.Sprintf(
		"that label would reuse the key %q of the field labelled %q on %s; choose a different label",
		heldSlug, heldLabel, object)}
}

// LabelTakenError refuses a label another field of the record type holds, or
// one whose key it would reuse. It unwraps to ErrConflict, so it answers 409.
type LabelTakenError struct{ message string }

func (e *LabelTakenError) Error() string { return e.message }

func (e *LabelTakenError) Unwrap() error { return apperrors.ErrConflict }

// MessageFault names the clash, so a screen can tell it from a busy table or a
// retired field.
func (e *LabelTakenError) MessageFault() (code, message string) {
	return codeLabelTaken, e.message
}

// codeLabelTaken is the code of every clash a different label resolves.
const codeLabelTaken = "custom_field_label_taken"

// optionsJSON renders a picklist's option set for the jsonb catalog
// column; non-picklist fields store NULL (a nil []byte binds as SQL
// NULL), not an empty array.
func optionsJSON(spec FieldSpec) ([]byte, error) {
	if !hasOptions(spec.Type) && spec.Type != TypeRecord {
		return nil, nil
	}
	return marshalOptions(spec.Options)
}

// marshalOptions is the one spelling of the options jsonb encoding.
func marshalOptions(options []string) ([]byte, error) {
	raw, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("customfields: encoding options: %w", err)
	}
	return raw, nil
}

// unmarshalOptions is the one spelling of the decoding, and it exists as a pair
// with marshalOptions because both catalog reads want the same answer to the same
// two questions: what a malformed column says to the caller, and what an absent
// option set decodes to. A second decode written at its own call site answered
// them differently once already.
//
// Empty raw is an empty set, not an error: a non-picklist field stores SQL NULL
// (optionsJSON), which is the normal shape for most rows rather than a fault.
func unmarshalOptions(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var options []string
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, fmt.Errorf("customfields: catalog options column is not a JSON string array: %w", err)
	}
	return options, nil
}

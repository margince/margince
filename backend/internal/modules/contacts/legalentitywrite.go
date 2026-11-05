// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// An administrator describing, changing and archiving a legal entity. Every
// write commits the row, its audit entry and its event in one transaction.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	legalEntityTable = "legal_entity"
	codeNameTaken    = "legal_entity_name_taken"
	actionArchive    = "archive"
	actionAssign     = "assign"
)

// LegalEntityRefusal is a write the entity's state refuses: the request was
// well formed, and retrying it unchanged cannot succeed.
type LegalEntityRefusal struct{ code, message string }

func (e *LegalEntityRefusal) Error() string { return e.message }

// MessageFault names the condition; no input of the request is wrong.
func (e *LegalEntityRefusal) MessageFault() (code, message string) { return e.code, e.message }

// Unwrap answers 409: the entity's state, not the request, is what refuses.
func (e *LegalEntityRefusal) Unwrap() error { return apperrors.ErrConflict }

var errNameTaken = &LegalEntityRefusal{
	codeNameTaken,
	"another live legal entity already has this name; choose a different one",
}

// CreateLegalEntity describes a further entity. display_name, legal_name and
// country are required.
func (s *Store) CreateLegalEntity(ctx context.Context, in LegalEntityInput) (LegalEntity, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionCreate); err != nil {
		return LegalEntity{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return LegalEntity{}, err
	}
	id := ids.NewV7()
	columns, args := []string{"id", "captured_by"}, []any{id, by}
	after := map[string]any{}
	for _, field := range in.fields() {
		value, _, err := field.cleaned(false)
		if err != nil {
			return LegalEntity{}, err
		}
		if value == nil && field.required {
			return LegalEntity{}, requiredFieldError(field.wire)
		}
		if value != nil {
			columns, args = append(columns, field.column), append(args, value)
			after[field.column] = value
		}
	}
	var out LegalEntity
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := refuseTheDefaultsName(ctx, tx, after[companyNameColumn]); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO legal_entity (%s) VALUES (%s)`,
			strings.Join(columns, ", "), storekit.Placeholders(args)), args...)
		if storekit.IsUniqueViolation(err) {
			return errNameTaken
		}
		if err != nil {
			return fmt.Errorf("insert legal entity: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, actionCreate, legalEntityObject, id, nil, after)
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.InternalEventLegalEntityCreated{
			DisplayName: strings.TrimSpace(*in.DisplayName),
		}); err != nil {
			return fmt.Errorf("emit legal_entity.created: %w", err)
		}
		out, err = readLegalEntity(ctx, tx, ids.From[ids.LegalEntityKind](id))
		return err
	})
	return out, err
}

// UpdateLegalEntity changes the fields a submission sends. On the default
// entity the anchor-held fields are refused: the company form edits them.
func (s *Store) UpdateLegalEntity(ctx context.Context, id ids.LegalEntityID, in LegalEntityInput, ifVersion *int64) (LegalEntity, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionUpdate); err != nil {
		return LegalEntity{}, err
	}
	var out LegalEntity
	err := s.tx(ctx, func(tx pgx.Tx) error {
		lock, err := storekit.LockRow(ctx, tx, legalEntityTable, id.UUID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, legalEntityTable, id.UUID, ifVersion); err != nil {
			return err
		}
		before, err := readLegalEntity(ctx, tx, id)
		if err != nil {
			return err
		}
		p, changed, err := legalEntityPatch(before, in)
		if err != nil {
			return err
		}
		if p.Empty() {
			out = before
			return nil
		}
		if err := refuseTheDefaultsName(ctx, tx, p.After()[companyNameColumn]); err != nil {
			return err
		}
		if err := p.ApplyLocked(ctx, tx, lock); storekit.IsUniqueViolation(err) {
			return errNameTaken
		} else if err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, actionUpdate, legalEntityObject, id.UUID, p.Before(), p.After())
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID,
			crmcontracts.InternalEventLegalEntityUpdated{ChangedFields: changed}); err != nil {
			return fmt.Errorf("emit legal_entity.updated: %w", err)
		}
		out, err = readLegalEntity(ctx, tx, id)
		return err
	})
	return out, err
}

// legalEntityPatch builds the change and names the fields it moves, in the
// contract's spelling, sorted so the event reads the same on every run.
func legalEntityPatch(before LegalEntity, in LegalEntityInput) (*storekit.Patch, []string, error) {
	current := before.asInput().fields()
	p := storekit.NewPatch()
	var changed []string
	for i, field := range in.fields() {
		value, sent, err := field.cleaned(before.IsDefault)
		if err != nil {
			return nil, nil, err
		}
		stored := current[i].value
		if !sent || sameStored(stored, value) {
			continue
		}
		p.Set(field.column, stored, value)
		changed = append(changed, field.wire)
	}
	sort.Strings(changed)
	return p, changed, nil
}

// sameStored compares a stored column with the value a patch would write.
func sameStored(stored, value *string) bool {
	if stored == nil || value == nil {
		return stored == value
	}
	return *stored == *value
}

// ArchiveLegalEntity retires an entity nobody works for any more. The default
// entity and an entity with members are refused.
func (s *Store) ArchiveLegalEntity(ctx context.Context, id ids.LegalEntityID, ifVersion *int64) (LegalEntity, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionDelete); err != nil {
		return LegalEntity{}, err
	}
	var out LegalEntity
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// The lock is what an assignment waits on too, so no user can join the
		// entity between the count below and the archive.
		if _, err := storekit.LockRow(ctx, tx, legalEntityTable, id.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, legalEntityTable, id.UUID, ifVersion); err != nil {
			return err
		}
		before, err := readLegalEntity(ctx, tx, id)
		if err != nil {
			return err
		}
		if before.IsDefault {
			return &LegalEntityRefusal{
				"legal_entity_default_is_permanent",
				"the default legal entity cannot be archived; it is the one every unassigned user works for",
			}
		}
		if before.MemberCount > 0 {
			return &LegalEntityRefusal{
				"legal_entity_has_members",
				fmt.Sprintf("%d users still work for this legal entity; move them to another entity first", before.MemberCount),
			}
		}
		var judged int
		var countArgs legalEntityArgs
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM company WHERE assess_against_entity_id = `+countArgs.add(id),
			countArgs...).Scan(&judged); err != nil {
			return fmt.Errorf("count the companies judged against a legal entity: %w", err)
		}
		if judged > 0 {
			return &LegalEntityRefusal{
				"legal_entity_is_assessed_against",
				fmt.Sprintf("%d companies are judged against this legal entity's offering; choose another offering for them first", judged),
			}
		}
		var args legalEntityArgs
		if _, err := tx.Exec(ctx, `UPDATE legal_entity SET archived_at = now() WHERE id = `+args.add(id), args...); err != nil {
			return fmt.Errorf("archive legal entity: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, actionArchive, legalEntityObject, id.UUID, nil, nil)
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.InternalEventLegalEntityArchived{}); err != nil {
			return fmt.Errorf("emit legal_entity.archived: %w", err)
		}
		out, err = readLegalEntity(ctx, tx, id)
		return err
	})
	return out, err
}

// refuseTheDefaultsName refuses a name the default entity already answers to,
// which is the anchor company's. uq_legal_entity_live_name cannot see it,
// because the default stores no name, so the comparison here uses the index's
// lower().
//
//craft:ignore naked-any a patch's after-image holds each column as the value it binds
func refuseTheDefaultsName(ctx context.Context, tx pgx.Tx, name any) error {
	proposed, ok := name.(*string)
	if !ok || proposed == nil {
		return nil
	}
	var anchor EntityIdentity
	if err := mergeAnchorIdentity(ctx, tx, &anchor); err != nil || anchor.DisplayName == "" {
		return err
	}
	var taken bool
	var args legalEntityArgs
	if err := tx.QueryRow(ctx, `SELECT lower(`+args.add(*proposed)+`) = lower(`+args.add(anchor.DisplayName)+`)`,
		args...).Scan(&taken); err != nil {
		return fmt.Errorf("compare a legal entity name with the company's: %w", err)
	}
	if taken {
		return errNameTaken
	}
	return nil
}

func requiredFieldError(wire string) error {
	_, _, err := entityField{wire: wire, value: new(string), required: true}.cleaned(false)
	return err
}

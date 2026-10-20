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

var errNameTaken = &LegalEntityRefusal{codeNameTaken,
	"another live legal entity already has this name; choose a different one"}

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
	current := before.storedValues()
	p := storekit.NewPatch()
	var changed []string
	for _, field := range in.fields() {
		value, sent, err := field.cleaned(before.IsDefault)
		if err != nil {
			return nil, nil, err
		}
		if !sent || sameStored(current[field.column], value) {
			continue
		}
		p.Set(field.column, current[field.column], value)
		changed = append(changed, field.wire)
	}
	sort.Strings(changed)
	return p, changed, nil
}

// sameStored compares a stored column with the value a patch would write.
func sameStored(stored *string, value any) bool {
	if stored == nil {
		return value == nil
	}
	text, ok := value.(string)
	return ok && text == *stored
}

// storedValues is each patchable column's current value. On the default
// entity the anchor-held ones are the anchor's, which no patch reaches.
func (e LegalEntity) storedValues() map[string]*string {
	name, a := e.DisplayName, e.Address
	return map[string]*string{
		"display_name": &name, "legal_name": e.LegalName,
		"address_line1": a.Line1, "address_line2": a.Line2, "address_postal_code": a.PostalCode,
		"address_city": a.City, "address_region": a.Region,
		"vat_id": e.VatID, "register_court": e.RegisterCourt, "register_number": e.RegisterNumber,
		"country": e.Country, "default_language": e.DefaultLanguage,
		"bank_account_holder": e.BankAccountHolder, "iban": e.IBAN, "bic": e.BIC, "imprint": e.Imprint,
	}
}

// ArchiveLegalEntity retires an entity nobody works for any more. The default
// entity and an entity with members are refused.
func (s *Store) ArchiveLegalEntity(ctx context.Context, id ids.LegalEntityID) (LegalEntity, error) {
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
		before, err := readLegalEntity(ctx, tx, id)
		if err != nil {
			return err
		}
		if before.IsDefault {
			return &LegalEntityRefusal{"legal_entity_default_is_permanent",
				"the default legal entity cannot be archived; it is the one every unassigned user works for"}
		}
		if before.MemberCount > 0 {
			return &LegalEntityRefusal{"legal_entity_has_members",
				fmt.Sprintf("%d users still work for this legal entity; move them to another entity first", before.MemberCount)}
		}
		if _, err := tx.Exec(ctx, `UPDATE legal_entity SET archived_at = now() WHERE id = $1`, id); err != nil {
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

func requiredFieldError(wire string) error {
	_, _, err := entityField{wire: wire, value: new(string), required: true}.cleaned(false)
	return err
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The legal entities the installation trades as, and the one resolver every
// other module reads an entity's identity through.
//
// The default entity is the anchor company seen as an issuer. It stores none of
// the anchor's fields, which legal_entity_default_reads_the_anchor holds. So the
// company form stays their only editor, and an installation with one entity
// behaves as it did before entities existed.

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

// legalEntityObject is the RBAC object every entity read and write asks.
const legalEntityObject = "legal_entity"

// LegalEntityAddress is the registered address. Its country is the entity's.
type LegalEntityAddress struct {
	Line1, Line2, PostalCode, City, Region *string
}

// LegalEntityOffering is what a further entity says it sells and to whom. The
// default entity's offering is the anchor company's profile, so these stay nil
// there.
type LegalEntityOffering struct {
	OfferSummary, ICP, ValueProposition, USP *string
}

// EntityIdentity is what a document, page or mail says about the entity behind
// it. On the default entity the anchor-held fields are the anchor's.
type EntityIdentity struct {
	ID        ids.LegalEntityID
	IsDefault bool
	// DisplayName is empty on the default entity of an installation that has not
	// described its company yet.
	DisplayName    string
	LegalName      *string
	Address        LegalEntityAddress
	VatID          *string
	RegisterCourt  *string
	RegisterNumber *string
	LogoObjectKey  *string
	// AnchorCompanyID is the company whose logo the default entity wears, and
	// the zero id on every other entity.
	AnchorCompanyID ids.CompanyID
	// Country is nil on the default entity when it takes the installation's.
	Country           *string
	DefaultLanguage   *string
	BankAccountHolder *string
	IBAN              *string
	BIC               *string
	Imprint           *string
	Offering          LegalEntityOffering
}

// LegalEntity is one entity as the settings screen lists it.
type LegalEntity struct {
	EntityIdentity
	// MemberCount counts assignment rows, so it is 0 on the default entity.
	MemberCount int
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

// legalEntityArgs collects one statement's arguments and answers each one's
// placeholder, so a $N is derived from the list rather than typed.
type legalEntityArgs []any

//craft:ignore naked-any SQL values cross this boundary with their driver types intact
func (a *legalEntityArgs) add(value any) string {
	*a = append(*a, value)
	return fmt.Sprintf("$%d", len(*a))
}

const legalEntityColumns = `id, is_default, coalesce(display_name, ''), legal_name,
	address_line1, address_line2, address_postal_code, address_city, address_region,
	vat_id, register_court, register_number, logo_object_key, country, default_language,
	bank_account_holder, iban, bic, imprint, offer_summary, icp, value_proposition, usp, version, created_at, updated_at, archived_at,
	(SELECT count(*) FROM legal_entity_member m WHERE m.legal_entity_id = legal_entity.id)`

func scanLegalEntity(row pgx.Row) (LegalEntity, error) {
	var out LegalEntity
	e := &out.EntityIdentity
	err := row.Scan(&e.ID, &e.IsDefault, &e.DisplayName, &e.LegalName,
		&e.Address.Line1, &e.Address.Line2, &e.Address.PostalCode, &e.Address.City, &e.Address.Region,
		&e.VatID, &e.RegisterCourt, &e.RegisterNumber, &e.LogoObjectKey, &e.Country, &e.DefaultLanguage,
		&e.BankAccountHolder, &e.IBAN, &e.BIC, &e.Imprint,
		&e.Offering.OfferSummary, &e.Offering.ICP, &e.Offering.ValueProposition, &e.Offering.USP, &out.Version, &out.CreatedAt, &out.UpdatedAt,
		&out.ArchivedAt, &out.MemberCount)
	return out, err
}

// readLegalEntity reads one entity, archived or live, with the anchor merged
// into the default.
func readLegalEntity(ctx context.Context, tx pgx.Tx, id ids.LegalEntityID) (LegalEntity, error) {
	var args legalEntityArgs
	out, err := scanLegalEntity(tx.QueryRow(ctx,
		`SELECT `+legalEntityColumns+` FROM legal_entity WHERE id = `+args.add(id), args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return LegalEntity{}, apperrors.ErrNotFound
	}
	if err != nil {
		return LegalEntity{}, fmt.Errorf("read legal entity: %w", err)
	}
	if out.IsDefault {
		err = mergeAnchorIdentity(ctx, tx, &out.EntityIdentity)
	}
	return out, err
}

// ResolveEntityIdentity resolves one entity's identity inside the caller's
// transaction, with the default entity's fields read from the anchor company.
//
// No grant is asked, for the reason ConfirmedIssuerLegalName gives. What the
// installation calls itself on its own documents, every seat already reads off
// them. An archived entity still resolves, because a document issued
// under it still names it.
func ResolveEntityIdentity(ctx context.Context, tx pgx.Tx, id ids.LegalEntityID) (EntityIdentity, error) {
	entity, err := readLegalEntity(ctx, tx, id)
	if err != nil {
		return EntityIdentity{}, err
	}
	return entity.EntityIdentity, nil
}

// mergeAnchorIdentity fills the default entity's anchor-held fields. An
// installation with no anchor company yet leaves them empty.
//
// The legal name is the CONFIRMED one: an offer prints it, and
// ConfirmedIssuerLegalName says why a proposal nobody confirmed must not be.
func mergeAnchorIdentity(ctx context.Context, tx pgx.Tx, e *EntityIdentity) error {
	companyID, err := anchorCompany(ctx, tx, false)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	e.AnchorCompanyID = companyID
	var args legalEntityArgs
	if err := tx.QueryRow(ctx,
		`SELECT display_name, address_line1, address_line2, address_postal_code,
		        address_city, address_region, logo_object_key
		   FROM company WHERE id = `+args.add(companyID), args...).Scan(&e.DisplayName,
		&e.Address.Line1, &e.Address.Line2, &e.Address.PostalCode, &e.Address.City,
		&e.Address.Region, &e.LogoObjectKey); err != nil {
		return fmt.Errorf("read the anchor company for the default legal entity: %w", err)
	}
	legalName, err := ConfirmedIssuerLegalName(ctx, tx)
	if err != nil {
		return err
	}
	if legalName != "" {
		e.LegalName = &legalName
	}
	return mergeAnchorRegister(ctx, tx, companyID, e)
}

// mergeAnchorRegister reads the three register facts the company form keeps as
// profile fields rather than columns.
func mergeAnchorRegister(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, e *EntityIdentity) error {
	var args legalEntityArgs
	rows, err := tx.Query(ctx,
		`SELECT field, value FROM company_profile_field
		  WHERE company_id = `+args.add(companyID)+
			` AND field = ANY(`+args.add([]string{fieldRegisterVat, fieldRegisterCourt, fieldRegisterNumber})+`)`,
		args...)
	if err != nil {
		return fmt.Errorf("read the anchor company's register fields: %w", err)
	}
	defer rows.Close()
	targets := map[string]**string{
		fieldRegisterVat: &e.VatID, fieldRegisterCourt: &e.RegisterCourt, fieldRegisterNumber: &e.RegisterNumber,
	}
	for rows.Next() {
		var field, value string
		if err := rows.Scan(&field, &value); err != nil {
			return fmt.Errorf("scan an anchor register field: %w", err)
		}
		if value != "" {
			*targets[field] = &value
		}
	}
	return rows.Err()
}

// UserLegalEntity is the entity a user works for: the one their assignment
// names, or the default entity when they have none.
func UserLegalEntity(ctx context.Context, tx pgx.Tx, user ids.UserID) (ids.LegalEntityID, error) {
	var id ids.LegalEntityID
	var args legalEntityArgs
	err := tx.QueryRow(ctx,
		`SELECT coalesce(
		    (SELECT legal_entity_id FROM legal_entity_member WHERE user_id = `+args.add(user)+`),
		    (SELECT id FROM legal_entity WHERE is_default))`, args...).Scan(&id)
	if err != nil {
		return ids.LegalEntityID{}, fmt.Errorf("resolve the legal entity of a user: %w", err)
	}
	return id, nil
}

// ListLegalEntities lists the entities, the default first and then by name.
func (s *Store) ListLegalEntities(ctx context.Context, includeArchived bool) ([]LegalEntity, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []LegalEntity
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args legalEntityArgs
		rows, err := tx.Query(ctx, `SELECT `+legalEntityColumns+` FROM legal_entity
		  WHERE `+args.add(includeArchived)+` OR archived_at IS NULL
		  ORDER BY is_default DESC, lower(display_name), id`, args...)
		if err != nil {
			return fmt.Errorf("list legal entities: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (LegalEntity, error) {
			return scanLegalEntity(row)
		})
		if err != nil {
			return fmt.Errorf("scan legal entities: %w", err)
		}
		for i := range out {
			if out[i].IsDefault {
				if err := mergeAnchorIdentity(ctx, tx, &out[i].EntityIdentity); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return out, err
}

// GetLegalEntity reads one entity, archived or live.
func (s *Store) GetLegalEntity(ctx context.Context, id ids.LegalEntityID) (LegalEntity, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionRead); err != nil {
		return LegalEntity{}, err
	}
	var out LegalEntity
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readLegalEntity(ctx, tx, id)
		return err
	})
	return out, err
}

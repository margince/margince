// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The legal entity an offer is issued by, and what the document says about it.
// A draft reads its entity live. A sent offer reads the issuer_snapshot its
// send froze, so a later change to the entity leaves the sent document alone.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// IssuerAddress is a legal entity's registered address.
type IssuerAddress struct {
	Line1, Line2, PostalCode, City, Region string
}

// IssuerEntity is what an offer prints about the legal entity behind it. On
// the default entity the anchor company's values stand in for the entity's.
type IssuerEntity struct {
	ID        ids.LegalEntityID
	IsDefault bool
	// Live is false on an archived entity, which no offer may newly name.
	Live                                 bool
	DisplayName, LegalName, Country      string
	Address                              IssuerAddress
	VatID, RegisterCourt, RegisterNumber string
	BankAccountHolder, IBAN, BIC         string
	LogoKey                              string
}

// AuthorEntity answers the legal entity a user works for.
type AuthorEntity func(ctx context.Context, tx pgx.Tx, user ids.UserID) (ids.LegalEntityID, error)

// IssuerEntityReader resolves one legal entity inside the caller's
// transaction; a nil id is the default entity.
type IssuerEntityReader func(ctx context.Context, tx pgx.Tx, id *ids.LegalEntityID) (IssuerEntity, error)

// OfferIssuer is the issuer block a rendered offer prints.
type OfferIssuer struct {
	Name                                 string
	Address                              []string
	VatID, RegisterCourt, RegisterNumber string
	BankAccountHolder, IBAN, BIC         string
	// LogoKey is the stored logo the document embeds, empty for none.
	LogoKey string
}

// The issuer_snapshot keys a send freezes. workspace_name keeps its name
// because offers sent before entities existed carry it as their whole issuer.
const (
	issuerKeyName           = "workspace_name"
	issuerKeyBaseCurrency   = "base_currency"
	issuerKeyEntity         = "legal_entity_id"
	issuerKeyDisplayName    = "display_name"
	issuerKeyLegalName      = "legal_name"
	issuerKeyAddress        = "address"
	issuerKeyCountry        = "country"
	issuerKeyVatID          = "vat_id"
	issuerKeyRegisterCourt  = "register_court"
	issuerKeyRegisterNumber = "register_number"
	issuerKeyBankHolder     = "bank_account_holder"
	issuerKeyIBAN           = "iban"
	issuerKeyBIC            = "bic"
	issuerKeyLogo           = "logo_object_key"
	issuerKeyLine1          = "line1"
	issuerKeyLine2          = "line2"
	issuerKeyPostalCode     = "postal_code"
	issuerKeyCity           = "city"
	issuerKeyRegion         = "region"
)

func errEntitySeamUnwired(seam string) error {
	return errors.New("deals: the " + seam + " seam was not injected; " +
		"construct this store with installseam.Deals(), which binds modules/contacts to it")
}

// offerAuthor is the human an offer is written for: the signed-in user, or the
// human an agent acts for. A caller with neither has no entity of its own, and
// its offer is issued by the default entity.
func offerAuthor(ctx context.Context) ids.UserID {
	actor, ok := principal.Actor(ctx)
	switch {
	case !ok:
		return ids.UserID{}
	case actor.Type == principal.PrincipalHuman:
		return ids.From[ids.UserKind](actor.UserID)
	default:
		return ids.From[ids.UserKind](actor.OnBehalfOf)
	}
}

// offerEntity is the entity a new or edited offer names: the one asked for,
// which must be live, else the author's.
func (s *Store) offerEntity(ctx context.Context, tx pgx.Tx, asked *ids.LegalEntityID) (ids.LegalEntityID, error) {
	if asked == nil {
		author := offerAuthor(ctx)
		if author.IsZero() {
			entity, err := s.installation.IssuerEntity(ctx, tx, nil)
			return entity.ID, err
		}
		return s.installation.AuthorEntity(ctx, tx, author)
	}
	entity, err := s.installation.IssuerEntity(ctx, tx, asked)
	if errors.Is(err, apperrors.ErrNotFound) || (err == nil && !entity.Live) {
		return ids.LegalEntityID{}, auth.NameMissingReference("legal_entity_id", apperrors.ErrNotFound)
	}
	return entity.ID, err
}

// issuerOf is the name a document is issued under. The default entity keeps
// the rule offers had before entities: the confirmed legal name, else the
// installation's name. A further entity always has a legal name.
func (s *Store) issuerOf(ctx context.Context, tx pgx.Tx, entity IssuerEntity) (string, error) {
	if entity.IsDefault {
		return s.issuerName(ctx, tx)
	}
	if entity.LegalName != "" {
		return entity.LegalName, nil
	}
	return entity.DisplayName, nil
}

// liveIssuer resolves the entity an unsent offer names, and its name.
func (s *Store) liveIssuer(ctx context.Context, tx pgx.Tx, offer crmcontracts.Offer) (IssuerEntity, string, error) {
	entity, err := s.installation.IssuerEntity(ctx, tx, legalEntityIDOf(offer))
	if err != nil {
		return IssuerEntity{}, "", fmt.Errorf("resolve the offer's legal entity: %w", err)
	}
	name, err := s.issuerOf(ctx, tx, entity)
	if err != nil {
		return IssuerEntity{}, "", fmt.Errorf("resolve the offer's issuer name: %w", err)
	}
	return entity, name, nil
}

// resolveRenderIssuer is the issuer block a render prints: the frozen
// snapshot once sent, else the offer's entity read live. Both halves read the
// one resolver the send freezes from, so a draft and its sent copy agree.
func (s *Store) resolveRenderIssuer(ctx context.Context, tx pgx.Tx, offer crmcontracts.Offer) (OfferIssuer, error) {
	if offer.IssuerSnapshot != nil {
		if issuer, ok := issuerFromSnapshot(*offer.IssuerSnapshot); ok {
			return issuer, nil
		}
	}
	// A sent offer never reads the entity as it is now. One with no snapshot
	// predates snapshots, and issued under the installation's own name.
	if offer.Status != crmcontracts.OfferStatusDraft {
		name, err := s.issuerName(ctx, tx)
		return OfferIssuer{Name: name}, err
	}
	entity, name, err := s.liveIssuer(ctx, tx, offer)
	if err != nil {
		return OfferIssuer{}, err
	}
	issuer, named := issuerFromSnapshot(issuerSnapshot(entity, name, ""))
	if !named {
		// An installation that has not named itself yet prints no issuer.
		return OfferIssuer{}, nil
	}
	return issuer, nil
}

func legalEntityIDOf(offer crmcontracts.Offer) *ids.LegalEntityID {
	if offer.LegalEntityId == nil {
		return nil
	}
	id := ids.From[ids.LegalEntityKind](ids.UUID(*offer.LegalEntityId))
	return &id
}

// issuerSnapshot is what a send freezes about the issuer. A field the entity
// has no value for is left out rather than stored empty.
//
//craft:ignore naked-any the snapshot is the jsonb object the contract documents
func issuerSnapshot(entity IssuerEntity, name, baseCurrency string) map[string]any {
	out := map[string]any{issuerKeyName: name, issuerKeyEntity: entity.ID.String()}
	if baseCurrency != "" {
		out[issuerKeyBaseCurrency] = baseCurrency
	}
	put := func(into map[string]any, key, value string) {
		if value != "" {
			into[key] = value
		}
	}
	for key, value := range map[string]string{
		issuerKeyDisplayName: entity.DisplayName, issuerKeyLegalName: entity.LegalName,
		issuerKeyCountry: entity.Country, issuerKeyVatID: entity.VatID,
		issuerKeyRegisterCourt: entity.RegisterCourt, issuerKeyRegisterNumber: entity.RegisterNumber,
		issuerKeyBankHolder: entity.BankAccountHolder, issuerKeyIBAN: entity.IBAN, issuerKeyBIC: entity.BIC,
		issuerKeyLogo: entity.LogoKey,
	} {
		put(out, key, value)
	}
	address := map[string]any{}
	a := entity.Address
	for key, value := range map[string]string{
		issuerKeyLine1: a.Line1, issuerKeyLine2: a.Line2, issuerKeyPostalCode: a.PostalCode,
		issuerKeyCity: a.City, issuerKeyRegion: a.Region,
	} {
		put(address, key, value)
	}
	if len(address) > 0 {
		out[issuerKeyAddress] = address
	}
	return out
}

// issuerFromSnapshot reads the block a snapshot froze. ok is false when the
// snapshot names no issuer at all. One sent before offers named their entity
// carries the name alone, and prints the name alone.
//
//craft:ignore naked-any the snapshot is the jsonb object the contract documents
func issuerFromSnapshot(snapshot map[string]any) (OfferIssuer, bool) {
	text := func(from map[string]any, key string) string {
		value, _ := from[key].(string)
		return value
	}
	name := text(snapshot, issuerKeyName)
	if name == "" {
		return OfferIssuer{}, false
	}
	address, _ := snapshot[issuerKeyAddress].(map[string]any)
	return OfferIssuer{
		Name: name,
		Address: addressLines(text(address, issuerKeyLine1), text(address, issuerKeyLine2),
			text(address, issuerKeyPostalCode), text(address, issuerKeyCity), text(address, issuerKeyRegion),
			text(snapshot, issuerKeyCountry)),
		VatID: text(snapshot, issuerKeyVatID), RegisterCourt: text(snapshot, issuerKeyRegisterCourt),
		RegisterNumber: text(snapshot, issuerKeyRegisterNumber), BankAccountHolder: text(snapshot, issuerKeyBankHolder),
		IBAN: text(snapshot, issuerKeyIBAN), BIC: text(snapshot, issuerKeyBIC), LogoKey: text(snapshot, issuerKeyLogo),
	}, true
}

// addressLines lays an address out as a letterhead does: street lines, then
// postal code and city, then region and country.
func addressLines(line1, line2, postalCode, city, region, country string) []string {
	var lines []string
	for _, line := range []string{line1, line2, joinNonEmpty(" ", postalCode, city), joinNonEmpty(", ", region, strings.ToUpper(country))} {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}

// sendSnapshots captures the buyer and issuer legal blocks at send time:
// the sent document stays truthful even when the company or workspace is
// later renamed.
func (s *Store) sendSnapshots(ctx context.Context, tx pgx.Tx, baseCurrency string,
	offer crmcontracts.Offer,
) (buyer, issuer map[string]any, err error) {
	if offer.BuyerCompanyId != nil {
		var displayName string
		var legalName *string
		err := tx.QueryRow(ctx,
			`SELECT display_name, legal_name FROM company WHERE id = $1`, offer.BuyerCompanyId).
			Scan(&displayName, &legalName)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, fmt.Errorf("snapshot buyer company: %w", err)
		}
		if err == nil {
			buyer = map[string]any{filterCompanyID: offer.BuyerCompanyId, "display_name": displayName}
			if legalName != nil {
				buyer["legal_name"] = *legalName
			}
		}
	}
	// The currency is the caller's resolved base, not a second read: the
	// snapshot must record the basis this offer was actually priced in.
	// The same resolver the render takes, so what a customer saw on the draft
	// is what the snapshot freezes as having been sent.
	entity, name, err := s.liveIssuer(ctx, tx, offer)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot the issuer: %w", err)
	}
	// An entity archived since the draft named it issues nothing new.
	if !entity.Live {
		return nil, nil, auth.NameMissingReference("legal_entity_id", apperrors.ErrNotFound)
	}
	return buyer, issuerSnapshot(entity, name, baseCurrency), nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sendingEntityOf is the legal entity whoever acts in ctx sends for, through
// the one resolver every reader of that question shares.
func sendingEntityOf(ctx context.Context, tx pgx.Tx) (ids.UUID, error) {
	return storekit.LegalEntityOf(ctx, tx, storekit.SenderOf(ctx))
}

// sendingEntityFacts is what data protection reads about the entity a message
// goes out for. The zero value is the default entity.
type sendingEntityFacts struct {
	isDefault bool
	found     bool
	country   string
	legalName string
	// address is the registered address as a block of lines, country last.
	address string
}

// further reports an entity other than the default, whose own particulars
// stand in for the installation's.
func (f sendingEntityFacts) further() bool { return f.found && !f.isDefault }

// sendingEntityFactsTx reads one entity, archived or live: a staged message
// keeps the entity it was resolved for. A zero id reads nothing.
func sendingEntityFactsTx(ctx context.Context, tx pgx.Tx, entity ids.UUID) (sendingEntityFacts, error) {
	if entity.IsZero() {
		return sendingEntityFacts{}, nil
	}
	var out sendingEntityFacts
	var country, legalName, line1, line2, postalCode, city, region *string
	args := []ids.UUID{entity}
	err := tx.QueryRow(ctx, `
		SELECT is_default, country, coalesce(legal_name, display_name),
		       address_line1, address_line2, address_postal_code, address_city, address_region
		  FROM legal_entity WHERE id = `+storekit.Placeholders(args), entity).
		Scan(&out.isDefault, &country, &legalName, &line1, &line2, &postalCode, &city, &region)
	if errors.Is(err, pgx.ErrNoRows) {
		return sendingEntityFacts{}, nil
	}
	if err != nil {
		return sendingEntityFacts{}, fmt.Errorf("consent: reading the sending legal entity: %w", err)
	}
	out.found = true
	out.country, out.legalName = deref(country), deref(legalName)
	out.address = joinStated("\n", deref(line1), deref(line2),
		joinStated(" ", deref(postalCode), deref(city)), deref(region), strings.ToUpper(out.country))
	return out, nil
}

// controllerOf is the installation's particulars with a further entity's own
// legal name and address in place of the installation's. The contacts and the
// objection route stay the installation's, because an entity states none.
func controllerOf(installation ControllerParticulars, entity sendingEntityFacts) ControllerParticulars {
	if !entity.further() {
		return installation
	}
	out := installation
	out.LegalName, out.PostalAddress = entity.legalName, entity.address
	return out
}

func joinStated(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}

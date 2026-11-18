// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// maxCountryLookups is the contract's bound: every address one message may carry.
const maxCountryLookups = 300

// RecipientCountryReading is the record's country for one address.
type RecipientCountryReading struct {
	Address string
	Country RecipientCountry
}

// RecipientCountries answers the country the marketing selector reads from the
// record for each address, which a composer shows beside the recipient. A
// record the caller may not read answers no country, so the answer names
// nothing about it.
func (s *Store) RecipientCountries(ctx context.Context, addresses []string) ([]RecipientCountryReading, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return nil, err
	}
	if len(addresses) == 0 || len(addresses) > maxCountryLookups {
		return nil, apperrors.ErrInvalidArgument
	}
	out := make([]RecipientCountryReading, 0, len(addresses))
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, address := range addresses {
			country, err := readableRecipientCountry(ctx, tx, strings.TrimSpace(address))
			if err != nil {
				return err
			}
			out = append(out, RecipientCountryReading{Address: address, Country: country})
		}
		return nil
	})
	return out, err
}

func readableRecipientCountry(ctx context.Context, tx pgx.Tx, address string) (RecipientCountry, error) {
	if address == "" {
		return RecipientCountry{}, nil
	}
	who, err := subjectOfRecipient(ctx, tx, connector.Recipient{Email: address})
	if err != nil || who.Kind == "" {
		return RecipientCountry{}, err
	}
	id, err := ids.Parse(who.ID)
	if err != nil {
		return RecipientCountry{}, err
	}
	err = auth.EnsureVisible(ctx, tx, who.Kind, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return RecipientCountry{}, nil
	}
	if err != nil {
		return RecipientCountry{}, err
	}
	return recipientCountryTx(ctx, tx, who)
}

// ResolveRecipientCountries serves POST /recipient-countries.
func (h Handlers) ResolveRecipientCountries(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.RecipientCountriesRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	readings, err := h.store.RecipientCountries(r.Context(), req.Addresses)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.RecipientCountries{Recipients: make([]crmcontracts.RecipientCountryReading, 0, len(readings))}
	for _, reading := range readings {
		entry := crmcontracts.RecipientCountryReading{Address: reading.Address}
		if reading.Country.Known() {
			code := string(reading.Country.Code)
			source := crmcontracts.RecipientCountryReadingSource(reading.Country.Source)
			entry.Country, entry.Source = &code, &source
		}
		out.Recipients = append(out.Recipients, entry)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Where the installation's default legal entity is established. It is the
// data-protection selector's answer for a sender with no entity of their own
// (ruleselectors.go). Marketing rules follow each recipient instead.
//
// The setting lives in identity, and a module never imports a sibling — so the
// reader is injected and this file holds only the seam.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// InstallationCountryReader answers where the installation is established, as a
// lower-case ISO 3166-1 alpha-2 code, or empty when it is unstated.
//
// It takes the CALLER'S transaction rather than opening its own, which is not
// incidental. The answer is stamped onto an immutable decision row: reading it
// outside the transaction that writes the row would let the setting change in
// between, and the row would record a decision taken under rules that were not
// the ones live when it was taken.
type InstallationCountryReader interface {
	InstallationCountryTx(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error)
}

// InstallationCountryFunc adapts a plain function, so compose can inject
// identity's reader without either module knowing the other.
type InstallationCountryFunc func(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error)

// InstallationCountryTx satisfies InstallationCountryReader.
func (f InstallationCountryFunc) InstallationCountryTx(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error) {
	return f(ctx, tx)
}

// WithInstallationCountry injects the reader behind the data-protection
// selector's default. It lands on the STORE, which both the send path and the
// request writers read through.
func (g *Gate) WithInstallationCountry(r InstallationCountryReader) *Gate {
	g.store.country = r
	return g
}

// WithInstallationCountry is the store-side spelling, for the surfaces that
// hold a store and no gate.
func (s *Store) WithInstallationCountry(r InstallationCountryReader) *Store {
	s.country = r
	return s
}

// WithInstallationCountry is the handlers-side spelling, for the request
// deadline the handlers preview.
func (h Handlers) WithInstallationCountry(r InstallationCountryReader) Handlers {
	h.store = h.store.WithInstallationCountry(r)
	return h
}

// defaultEntityCountry resolves the default entity's code, or empty when
// nothing is wired or nothing is set. Empty selects no pack: the GDPR month for
// request deadlines and no pack disclosures.
func (s *Store) defaultEntityCountry(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error) {
	if s.country == nil {
		return "", nil
	}
	return s.country.InstallationCountryTx(ctx, tx)
}

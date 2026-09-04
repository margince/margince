// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Where this installation is established, which is what selects the messaging
// rules an outbound decision is taken under.
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

// WithInstallationCountry injects the reader behind jurisdiction resolution.
//
// It lands on the STORE rather than the gate because two callers need the
// jurisdiction's windows and only one of them is a gate: the send path asks
// through the engine, and GET /people/{id}/consent/guard asks through the
// store. A preview that answered on a different window than the send would be
// the second answer to one question, which is the failure this repository
// spends most of its gates preventing.
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

// WithInstallationCountry is the handlers-side spelling. The consent guard
// endpoint reads the same windows the send path binds a decision to, so it
// needs the same jurisdiction.
func (h Handlers) WithInstallationCountry(r InstallationCountryReader) Handlers {
	h.store = h.store.WithInstallationCountry(r)
	return h
}

// installationCountry resolves the code, or empty when nothing is wired or
// nothing is set.
//
// An unwired reader answers empty rather than failing, and empty selects NO
// jurisdiction rules. That direction is the safe one for what currently reads
// this: the frequency cap is a ceiling a pack adds, so its absence leaves the
// consent requirement — which binds everywhere and is enforced before any pack
// is consulted — doing exactly what it did before. A reader added later that
// RELAXES a rule on the strength of a country must not use this, because it
// would then relax on an unstated one.
func (s *Store) installationCountry(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error) {
	if s.country == nil {
		return "", nil
	}
	return s.country.InstallationCountryTx(ctx, tx)
}

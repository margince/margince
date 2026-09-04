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
func (g *Gate) WithInstallationCountry(r InstallationCountryReader) *Gate {
	g.country = r
	return g
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
func (g *Gate) installationCountry(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error) {
	if g.country == nil {
		return "", nil
	}
	return g.country.InstallationCountryTx(ctx, tx)
}

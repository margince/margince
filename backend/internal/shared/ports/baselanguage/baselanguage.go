// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package baselanguage answers the installation's base language for
// shared-record text a module writes: a sentence stored once and read by every
// seat, such as an approval's summary, follows the installation rather than
// whoever happens to be reading it.
//
// The setting lives in identity, and a module never imports a sibling, so a
// module holds this port and compose injects what answers it.
package baselanguage

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Resolver answers the base language, and English when it cannot be read; the
// implementation logs that failure, because the caller sees only a language.
type Resolver func(ctx context.Context) textlang.Lang

// Resolve asks r, answering English for a nil Resolver: a composition that
// wires none writes what every installation read before the setting existed.
func (r Resolver) Resolve(ctx context.Context) textlang.Lang {
	if r == nil {
		return textlang.English
	}
	return r(ctx)
}

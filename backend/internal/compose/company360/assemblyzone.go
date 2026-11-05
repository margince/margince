// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// zone is the installation zone every day count on the page is reckoned in,
// read once per assembly like the language.
func (a *assembly) zone() (*time.Location, error) {
	if a.loc == nil {
		loc, err := identity.ZoneAppliedTx(a.ctx, a.tx)
		if err != nil {
			return nil, err
		}
		a.loc = loc
	}
	return a.loc, nil
}

// installationBaseCurrency resolves the installation's reporting currency once
// per assembly. The sections that label money each want the same answer, and
// the 360's query budget counts every read.
func (a *assembly) installationBaseCurrency() (string, error) {
	if a.baseCurrencyRead {
		return a.baseCurrency, nil
	}
	// A caller who may not read it is withheld the money sections, not the read.
	readCtx, err := auth.Unrecorded(a.ctx, identity.SettingsObject, principal.ActionRead)
	if err != nil {
		return "", err
	}
	base, err := identity.BaseCurrencyOf(readCtx, a.tx)
	if err != nil {
		return "", err
	}
	a.baseCurrency, a.baseCurrencyRead = base, true
	return base, nil
}

// One assembly uses one language, the reader's, even if the setting changes during its reads.
func (a *assembly) recordLanguage() textlang.Lang {
	if a.readerLanguage == "" {
		a.readerLanguage = identity.ReaderLanguageForRecord(a.ctx, a.tx)
	}
	return a.readerLanguage
}

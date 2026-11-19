// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Writing the greeting register: a plain column a rep sets or clears.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
)

// fieldGreetingRegister is the wire field and the column of the register.
const fieldGreetingRegister = "greeting_register"

// patchGreetingRegister stages a set register beside the other columns. A clear
// is staged by ApplyClears through clearableContactColumns.
func patchGreetingRegister(p *storekit.Patch, current crmcontracts.Contact, in UpdateContactInput) error {
	if in.GreetingRegister == nil {
		return nil
	}
	register := *in.GreetingRegister
	if !greetingregister.Valid(register) {
		return httperr.Validation(fieldGreetingRegister, "invalid", "unknown greeting register")
	}
	p.Set(fieldGreetingRegister, current.GreetingRegister, string(register))
	return nil
}

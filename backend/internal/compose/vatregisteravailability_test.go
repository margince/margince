// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

func TestMeSaysTheVatRegisterIsConsultedOnlyWhereTheCheckIsWired(t *testing.T) {
	bound := boundVatCheckEnqueue()
	t.Cleanup(func() { BindVatChecking(bound) })

	unwired := &Server{}
	if unwired.VatRegisterAvailable() {
		t.Error("/me says vat_register on a server that wired no VAT check")
	}

	wired := &Server{contactsStore: contacts.NewStore(nil), contactsHandlers: contacts.NewHandlers(nil)}
	WithVatChecking(insertOnlyRunnerForTest(t))(wired, nil)
	if !wired.VatRegisterAvailable() {
		t.Error("/me says no vat_register on a server whose VAT check is wired")
	}
}

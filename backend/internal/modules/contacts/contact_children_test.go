// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestANewAddressNeedsADottedDomain(t *testing.T) {
	err := parseContactContacts([]ContactEmailInput{{Email: "a@b", IsPrimary: true}}, nil)
	if _, refused := errors.AsType[*values.ParseError](err); !refused {
		t.Fatalf("a new contact took the address a@b: err = %v", err)
	}
}

func TestAnAddressTheContactAlreadyHoldsStaysWhenTheListChanges(t *testing.T) {
	emails := []ContactEmailInput{{Email: " Ops@Intranet ", IsPrimary: true}, {Email: "Ada@Example.com"}}
	held := map[string]bool{"ops@intranet": true}
	if err := parseContactHandles(emails, nil, held); err != nil {
		t.Fatalf("adding an address refused the one the contact already holds: %v", err)
	}
	if emails[0].Email != "ops@intranet" || emails[1].Email != "ada@example.com" {
		t.Fatalf("addresses = %q, %q, want both lowercased", emails[0].Email, emails[1].Email)
	}
	emails = append(emails, ContactEmailInput{Email: "new@intranet"})
	if err := parseContactHandles(emails, nil, held); err == nil {
		t.Fatal("an address the contact does not hold skipped the dotted-domain rule")
	}
}

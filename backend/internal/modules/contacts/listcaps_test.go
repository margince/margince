// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestAReplaceSetPastItsCapIsRefusedUnlessTheRecordAlreadyHoldsAsMany(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name        string
		sent, held  int
		wantRefused bool
	}{
		{name: "at the cap", sent: maxContactEmails},
		{name: "past the cap on a new record", sent: maxContactEmails + 1, wantRefused: true},
		{name: "past the cap and past what is held", sent: maxContactEmails + 3, held: maxContactEmails + 2, wantRefused: true},
		{name: "a held list past the cap sent back", sent: maxContactEmails + 2, held: maxContactEmails + 2},
		{name: "a held list past the cap made shorter", sent: maxContactEmails + 1, held: maxContactEmails + 2},
	} {
		err := refuseLongerList(contactEmailsField, "addresses", maxContactEmails, c.sent, c.held)
		if !c.wantRefused {
			if err != nil {
				t.Errorf("%s: %v, want admitted", c.name, err)
			}
			continue
		}
		detailed, ok := errors.AsType[*httperr.DetailedError](err)
		if !ok || len(detailed.Fields) != 1 || detailed.Fields[0].Field != contactEmailsField || detailed.Fields[0].Code != "too_many" {
			t.Errorf("%s: err = %v, want emails too_many", c.name, err)
		}
	}
}

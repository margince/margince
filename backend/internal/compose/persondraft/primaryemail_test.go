// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package persondraft

// Which address a message goes to.
//
// The cases here are the shared table the BROWSER's picker is held to as well:
// a case added on one side and not the other is the drift
// gates/frontendprimaryemail_test.go exists to catch. Written to be read from
// both languages — one list in, one address out.

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The four shapes an address on a record can have. Named rather than passed as
// two booleans: `address("x", false, true)` at a call site says nothing about
// which flag is which, and the whole table turns on telling them apart.
type addressShape int

const (
	plain addressShape = iota
	marked
	retired
	markedAndRetired
)

func address(email string, shape addressShape) crmcontracts.PersonEmail {
	out := crmcontracts.PersonEmail{
		Email:     openapi_types.Email(email),
		IsPrimary: shape == marked || shape == markedAndRetired,
	}
	if shape == retired || shape == markedAndRetired {
		when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		out.ArchivedAt = &when
	}
	return out
}

func personWith(emails ...crmcontracts.PersonEmail) crmcontracts.Person {
	if emails == nil {
		return crmcontracts.Person{}
	}
	return crmcontracts.Person{Emails: &emails}
}

func TestTheAddressAContactIsWrittenTo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		person crmcontracts.Person
		want   string
	}{
		{
			name: "takes the one marked primary",
			person: personWith(
				address("old@buyer.test", plain),
				address("anna@buyer.test", marked),
			),
			want: "anna@buyer.test",
		},
		{
			// An unmarked address is still reachable. A contact with exactly one
			// address and no flag set is the ordinary case, and refusing to write
			// to them would read the flag as permission when it only ranks.
			name: "takes the first live one when nothing is marked",
			person: personWith(
				address("anna@buyer.test", plain),
				address("anna.weber@buyer.test", plain),
			),
			want: "anna@buyer.test",
		},
		{
			// The one wrong answer here. Somebody retired that address: mail to
			// it either bounces or reaches a person who asked us to stop.
			name: "skips an archived address even when it is first",
			person: personWith(
				address("left@buyer.test", retired),
				address("anna@buyer.test", plain),
			),
			want: "anna@buyer.test",
		},
		{
			// Archived outranks primary: retirement is a decision about the
			// address itself, and the flag only ranks among live ones.
			name: "skips an archived address even when it is marked primary",
			person: personWith(
				address("left@buyer.test", markedAndRetired),
				address("anna@buyer.test", plain),
			),
			want: "anna@buyer.test",
		},
		{
			name:   "answers nothing when every address is archived",
			person: personWith(address("left@buyer.test", retired)),
			want:   "",
		},
		{
			name:   "answers nothing for a contact with no address at all",
			person: personWith(),
			want:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := primaryEmail(tc.person); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

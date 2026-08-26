// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
)

// What a lead is called decides two things at once: whether the promotion will
// take it at all, and which person the ladder then matches. The cases that
// matter are the ones where "has a name" and "has a full_name" part company —
// a pointer to an empty string is a full_name and is not a name.
func TestLeadIdentityName(t *testing.T) {
	email := func(s string) *openapi_types.Email {
		e := openapi_types.Email(s)
		return &e
	}
	for _, tc := range []struct {
		name string
		lead crmcontracts.Lead
		want string
	}{
		{
			name: "a name is the name",
			lead: crmcontracts.Lead{FullName: ptr("Vera Lindqvist")},
			want: "Vera Lindqvist",
		},
		{
			name: "the email names a lead that has no name of its own",
			lead: crmcontracts.Lead{Email: email("vera@nordwind.example")},
			want: "vera@nordwind.example",
		},
		{
			name: "a name wins over an email",
			lead: crmcontracts.Lead{FullName: ptr("Vera Lindqvist"), Email: email("vera@nordwind.example")},
			want: "Vera Lindqvist",
		},
		{
			name: "a present-but-empty full_name is not a name",
			lead: crmcontracts.Lead{FullName: ptr(""), Email: email("vera@nordwind.example")},
			want: "vera@nordwind.example",
		},
		{
			name: "padding is not a name either",
			lead: crmcontracts.Lead{FullName: ptr("   "), Email: email("vera@nordwind.example")},
			want: "vera@nordwind.example",
		},
		{
			name: "a name keeps no padding into the ladder",
			lead: crmcontracts.Lead{FullName: ptr("  Vera Lindqvist  ")},
			want: "Vera Lindqvist",
		},
		{
			name: "nothing names a lead with neither",
			lead: crmcontracts.Lead{},
			want: "",
		},
		{
			// The case the promotion guard has to refuse: a full_name that
			// exists, names nobody, and has no address standing behind it.
			name: "an empty full_name and no email names nobody",
			lead: crmcontracts.Lead{FullName: ptr("")},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := leadIdentityName(tc.lead); got != tc.want {
				t.Errorf("leadIdentityName() = %q, want %q", got, tc.want)
			}
		})
	}
}

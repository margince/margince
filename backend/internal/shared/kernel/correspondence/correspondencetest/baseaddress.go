// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package correspondencetest holds the addresses that pin
// correspondence.BaseAddress. The unit test asks the Go rule about them. The
// contacts integration test asks the SQL expression, so a case added for one
// side reaches the other.
package correspondencetest

// BaseAddressCase is one address and the base the duplicate check compares.
// Changed is whether the base differs from the folded address.
type BaseAddressCase struct {
	Address, Base string
	Changed       bool
}

// BaseAddressCases are the awkward inputs: tags, case, the two Gmail
// spellings, and hosts that must keep their dots.
var BaseAddressCases = []BaseAddressCase{
	{"Anna@Example.com", "anna@example.com", false},
	{"anna+Accounting@example.com", "anna@example.com", true},
	{"anna+a+b@example.com", "anna@example.com", true},
	{"AB+X@Example.COM", "ab@example.com", true},
	{"+tag@example.com", "+tag@example.com", false},
	{"ab@x+y.example.com", "ab@x+y.example.com", false},
	{"a.b+tag@sub.example.com", "a.b@sub.example.com", true},
	{"no-host", "no-host", false},
	{"A.B@Gmail.com", "ab@gmail.com", true},
	{"ab@gmail.com", "ab@gmail.com", false},
	{"a.b+crm@googlemail.com", "ab@gmail.com", true},
	{"ab@googlemail.com", "ab@gmail.com", true},
	{"+t.ag@gmail.com", "+tag@gmail.com", true},
	{"a.b@example.com", "a.b@example.com", false},
	{"a.b@outlook.com", "a.b@outlook.com", false},
	{"a.b@mail.gmail.com", "a.b@mail.gmail.com", false},
	{"a.b@gmail.com.example", "a.b@gmail.com.example", false},
	{"a.b@xgmail.com", "a.b@xgmail.com", false},
	{"a@b@gmail.com", "a@b@gmail.com", false},
	{"a.b@c@gmail.com", "a.b@c@gmail.com", false},
	{"a.b@gmail.com@example.com", "a.b@gmail.com@example.com", false},
}

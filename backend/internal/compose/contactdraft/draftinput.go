// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

// What the shared writer needs of this folded record, and nothing about what it
// is grounded in. Its own file rather than input.go's tail: the fold is a long
// read of a 360, and these six methods are a contract with draftcore.

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// The draftcore.Input contract: what the shared writer needs of a folded
// record, and nothing about what this one is grounded in.

// Fenced is what the model sees. The caller's own intent is removed because it
// rides the user turn OUTSIDE the fence — it is the one input they typed rather
// than untrusted text read off somebody's mail.
//
//craft:ignore naked-any the writer marshals this to JSON and reads nothing off it; the two surfaces return different structs, so a concrete type here would be one of them and exclude the other.
func (in Input) Fenced() any {
	in.Intent = ""
	return in
}

// WrittenInto is the correspondence this draft is written into.
//
// Not called Envelope: this struct already has a FIELD by that name, and the
// shared writer needs a method it can ask through an interface.
func (in Input) WrittenInto() draftfloor.Envelope { return in.Envelope }

// Steering is the caller's own words.
func (in Input) Steering() string { return in.Intent }

// GreetingNames are the two names a greeting-line repair looks for. Both,
// because the register decides which one opens the message.
func (in Input) GreetingNames() (string, string) {
	return in.Recipient.FirstName, in.Recipient.LastName
}

// CoRecipientNames are the further readers' names, which a greeting may name
// beside the recipient's.
func (in Input) CoRecipientNames() []string {
	names := make([]string, 0, len(in.AlsoTo))
	for _, other := range in.AlsoTo {
		names = append(names, other.Name)
	}
	return names
}

// Addresses is where the draft goes: the recipient's own, then each further
// recipient's, skipping a contact with no address and an address twice.
func (in Input) Addresses() []string {
	var out []string
	for _, address := range append([]string{in.Recipient.Email}, coAddresses(in.AlsoTo)...) {
		if address != "" && !slices.Contains(out, address) {
			out = append(out, address)
		}
	}
	return out
}

func coAddresses(others []CoRecipientIn) []string {
	out := make([]string, 0, len(others))
	for _, other := range others {
		out = append(out, other.Email)
	}
	return out
}

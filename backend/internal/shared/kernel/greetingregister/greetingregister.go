// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package greetingregister is the vocabulary of contact.greeting_register: how
// a message to one contact opens. A rep sets it, and a draft takes it before
// reading the thread.
package greetingregister

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Register is a value of contact.greeting_register.
//
// Held by: TestTheGreetingRegisterVocabularyIsTheCatalogsCheck
// (backend/gates/greetingregistervocabulary_test.go), which fails when this
// set and the column's CHECK stop naming the same values.
type Register string

const (
	// Formal opens with the surname.
	Formal Register = "formal"
	// Familiar opens with the first name.
	Familiar Register = "familiar"
)

// Values lists every register the column accepts. NULL is not among them: it
// means nobody set one.
func Values() []Register { return []Register{Formal, Familiar} }

// Valid reports whether r is a register the column accepts.
func Valid(r Register) bool { return slices.Contains(Values(), r) }

// Of reads a contract's greeting_register back as a Register, empty when none
// is set or the value is one this build does not know. A generic over the
// contract's own enum types, so each read model passes its own.
func Of[S ~string](stored *S) Register {
	if stored == nil || !Valid(Register(*stored)) {
		return ""
	}
	return Register(*stored)
}

// Shared is the register one message to several contacts takes. Registers a
// rep left unset say nothing. Where the set ones differ the message is formal,
// because a formal opening offends nobody who asked for a familiar one.
func Shared(each ...Register) Register {
	var chosen Register
	for _, register := range each {
		switch {
		case register == "":
		case chosen == "":
			chosen = register
		case chosen != register:
			return Formal
		}
	}
	return chosen
}

// Address is the German du or Sie a stored register fixes, and Unknown for
// anything else, which leaves the register to the correspondence.
func Address(r Register) textlang.Register {
	switch r {
	case Formal:
		return textlang.RegisterSie
	case Familiar:
		return textlang.RegisterDu
	}
	return textlang.RegisterUnknown
}

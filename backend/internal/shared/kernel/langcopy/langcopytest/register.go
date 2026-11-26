// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"slices"
	"strings"
)

// Register says who speaks a table and who reads it, which decides the voice
// and address rules it answers to. Member copy names none; any other register
// is named at the call site with the reason it applies.
type Register struct {
	name   string
	reason string
	// drops are the rules this register does not hold; keeps, when set, are
	// the only ones it holds.
	drops []string
	keeps []string
}

var member = Register{name: "member copy", drops: []string{ruleViOutsiderAddress}}

var mechanics = []string{
	ruleEnDash, ruleEnSpelling, ruleEnAmpersand,
	ruleViToneOpen, ruleViToneClosed, ruleViDash, ruleViExclamation, ruleViNFC, ruleViQuotes,
}

// Agent is an agent speaking in a chat bubble, which may say I.
func Agent(reason string) Register {
	return Register{name: "agent voice", reason: reason, drops: []string{ruleEnI, ruleViToi, ruleViOutsiderAddress}}
}

// Outsider is the product writing to a contact, a buyer or a booking guest,
// who is quý khách in Vietnamese and never bạn.
func Outsider(reason string) Register {
	return Register{name: "outsider copy", reason: reason, drops: []string{ruleViMemberAddress}}
}

// Controller is the data controller writing to a data subject. In law it
// speaks as we and chúng tôi, and it addresses an outsider.
func Controller(reason string) Register {
	return Register{
		name: "controller copy", reason: reason,
		drops: []string{ruleEnWe, ruleViChungToi, ruleViMemberAddress},
	}
}

// OwnVoice is a sentence someone speaks in their own words: a draft a member
// sends as themselves, or a consent the reader gives. Only mechanics hold.
func OwnVoice(reason string) Register {
	return Register{name: "own voice", reason: reason, keeps: mechanics}
}

// Verbatim is text a program matches or a reader types as it stands: a
// matcher's word list, a sample cell, an enum value. No style rule holds it.
func Verbatim(reason string) Register {
	return Register{name: "verbatim text", reason: reason, keeps: []string{}}
}

func (r Register) holds(rule string) bool {
	if r.keeps != nil {
		return slices.Contains(r.keeps, rule)
	}
	return !slices.Contains(r.drops, rule)
}

// named answers the register a call site chose, or member copy when it chose
// none. A register with no stated reason fails, so no table leaves member copy
// without saying why.
func named(t reporter, registers []Register) Register {
	t.Helper()
	switch {
	case len(registers) == 0:
		return member
	case len(registers) > 1:
		t.Fatalf("a table has one register, and this call names %d", len(registers))
	case registers[0].name == "":
		t.Fatalf("a zero Register names no voice; build it with Agent, Outsider, Controller, OwnVoice or Verbatim")
	case strings.TrimSpace(registers[0].reason) == "":
		t.Fatalf("the %s register names no reason; say who speaks this table and who reads it", registers[0].name)
	}
	return registers[0]
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The settings a unit declares as data. The core draws them on the unit's card
// of the Extensions page, keeps the values and audits every change, so a unit
// ships no settings screen of its own.

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// SettingKind is the control the core draws for a setting.
type SettingKind string

const (
	// SettingSwitch is an on/off choice, held as "true" or "false".
	SettingSwitch SettingKind = "switch"
	// SettingText is one short line.
	SettingText SettingKind = "text"
)

const (
	maxSettingLabel = 60
	maxSettingHint  = 200
	maxSettingText  = 200
)

// Setting is one value an admin can change for the unit.
type Setting struct {
	// Key names the setting inside the unit, in the unit-name grammar.
	Key string
	// Label and Hint are shown as written, in one language, like Description.
	Label string
	Hint  string
	Kind  SettingKind
}

// Validate checks what one declaration can show on its own.
func (s Setting) Validate() error {
	if !nameGrammar.MatchString(s.Key) {
		return fmt.Errorf("%w: setting key %q must be lower-case letters and digits joined by single hyphens", ErrInvalid, s.Key)
	}
	if s.Kind != SettingSwitch && s.Kind != SettingText {
		return fmt.Errorf("%w: setting %q has kind %q, want %q or %q", ErrInvalid, s.Key, s.Kind, SettingSwitch, SettingText)
	}
	if err := oneLine("label", s.Label, maxSettingLabel, true); err != nil {
		return fmt.Errorf("%w: setting %q: %v", ErrInvalid, s.Key, err)
	}
	if err := oneLine("hint", s.Hint, maxSettingHint, false); err != nil {
		return fmt.Errorf("%w: setting %q: %v", ErrInvalid, s.Key, err)
	}
	return nil
}

// Default is the value of a setting nobody has changed.
func (s Setting) Default() string {
	if s.Kind == SettingSwitch {
		return "false"
	}
	return ""
}

// CheckValue refuses a value the setting's control could not have produced.
func (s Setting) CheckValue(v string) error {
	if s.Kind == SettingSwitch {
		if v != "true" && v != "false" {
			return fmt.Errorf("%w: setting %q is a switch, so its value is true or false", ErrInvalid, s.Key)
		}
		return nil
	}
	if err := oneLine("value", v, maxSettingText, false); err != nil {
		return fmt.Errorf("%w: setting %q: %v", ErrInvalid, s.Key, err)
	}
	return nil
}

func oneLine(what, v string, limit int, required bool) error {
	if required && v == "" {
		return fmt.Errorf("the %s is empty", what)
	}
	for _, r := range v {
		if !unicode.IsPrint(r) {
			return fmt.Errorf("the %s holds a character that is not printable", what)
		}
	}
	if n := utf8.RuneCountInString(v); n > limit {
		return fmt.Errorf("the %s is %d characters, capped at %d", what, n, limit)
	}
	return nil
}

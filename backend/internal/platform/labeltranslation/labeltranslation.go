// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package labeltranslation holds the names an admin gives a label in further
// UI languages: a pipeline stage, a custom field, a picklist option. The
// stored label stays the fallback for a language with no entry.
package labeltranslation

import (
	"fmt"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// MaxLength bounds one translated label, the same bound a typed label meets
// in the settings forms.
const MaxLength = 200

// Error refuses a translation map, naming the request field it came from.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string { return e.Message }

// FieldFault places the refusal on the field that carried the map.
func (e *Error) FieldFault() (field, code, message string) {
	return e.Field, "invalid_translation", e.Message
}

// Normalize answers the map as stored: keys are shipped UI languages, values
// are trimmed, and an empty value drops its language. field names the request
// field for a refusal.
func Normalize(field string, in map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for lang, text := range in {
		if !textlang.Known(lang) {
			return nil, &Error{Field: field, Message: fmt.Sprintf(
				"%s names the language %q; use one of the UI languages en, de or vi", field, lang)}
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > MaxLength {
			return nil, &Error{Field: field, Message: fmt.Sprintf(
				"%s for %q is longer than %d characters", field, lang, MaxLength)}
		}
		out[lang] = text
	}
	return out, nil
}

// NormalizeOptions is Normalize for a picklist: one language map per option,
// and only for options the field offers.
func NormalizeOptions(field string, in map[string]map[string]string, options []string) (map[string]map[string]string, error) {
	offered := make(map[string]bool, len(options))
	for _, option := range options {
		offered[option] = true
	}
	out := make(map[string]map[string]string, len(in))
	for option, names := range in {
		if !offered[option] {
			return nil, &Error{Field: field, Message: fmt.Sprintf(
				"%s names the option %q, which this field does not offer", field, option)}
		}
		normalized, err := Normalize(field, names)
		if err != nil {
			return nil, err
		}
		if len(normalized) > 0 {
			out[option] = normalized
		}
	}
	return out, nil
}

// FromWire reads the contract's per-language object as a map. Nil reads as
// no translations.
func FromWire(in *crmcontracts.LabelTranslations) map[string]string {
	out := map[string]string{}
	if in == nil {
		return out
	}
	for lang, text := range map[string]*string{"en": in.En, "de": in.De, "vi": in.Vi} {
		if text != nil {
			out[lang] = *text
		}
	}
	return out
}

// ToWire writes a stored map in the contract's shape.
func ToWire(in map[string]string) *crmcontracts.LabelTranslations {
	out := crmcontracts.LabelTranslations{}
	for lang, text := range in {
		switch lang {
		case "en":
			out.En = &text
		case "de":
			out.De = &text
		case "vi":
			out.Vi = &text
		}
	}
	return &out
}

// OptionsFromWire reads the contract's option map.
func OptionsFromWire(in *crmcontracts.OptionTranslations) map[string]map[string]string {
	out := map[string]map[string]string{}
	if in == nil {
		return out
	}
	for option, names := range *in {
		out[option] = FromWire(&names)
	}
	return out
}

// OptionsToWire writes a stored option map in the contract's shape.
func OptionsToWire(in map[string]map[string]string) *crmcontracts.OptionTranslations {
	out := crmcontracts.OptionTranslations{}
	for option, names := range in {
		out[option] = *ToWire(names)
	}
	return &out
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The words a staged summary names a record type with, and the other pieces of
// vocabulary both doors of the approval surface share.
//
// The tool door writes its summaries here and the REST gate in compose writes
// its own, and two tables of one vocabulary drift until a German inbox shows
// "Kontakt" on one card and "contact" on the next. So this module owns the
// words, and compose reads them through the exported functions below.

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// recordNounsByLang is the record type as a reader of each language says it,
// keyed by the wire type. English has no entry: it reads the wire type aloud.
//
//nolint:goconst // the keys are the contract's wire types, one row per language; a constant per type would be a second spelling of them
var recordNounsByLang = map[textlang.Lang]map[string]string{
	textlang.German: {
		"activity":             systemtext.Copy("noun.activity", textlang.German),
		"company":              systemtext.Copy("noun.company", textlang.German),
		"contact":              systemtext.Copy("noun.contact", textlang.German),
		"custom_field":         systemtext.Copy("noun.custom_field", textlang.German),
		"deal":                 systemtext.Copy("noun.deal", textlang.German),
		"deal_room":            systemtext.Copy("noun.deal_room", textlang.German),
		"deal_room_comment":    systemtext.Copy("noun.deal_room_comment", textlang.German),
		"deal_room_thread":     systemtext.Copy("noun.deal_room_thread", textlang.German),
		"lead":                 systemtext.Copy("noun.lead", textlang.German),
		"offer":                systemtext.Copy("noun.offer", textlang.German),
		"offer_template":       systemtext.Copy("noun.offer_template", textlang.German),
		"product":              systemtext.Copy("noun.product", textlang.German),
		"project":              systemtext.Copy("noun.project", textlang.German),
		"relationship":         systemtext.Copy("noun.relationship", textlang.German),
		"saved_view":           systemtext.Copy("noun.saved_view", textlang.German),
		"tag":                  systemtext.Copy("noun.tag", textlang.German),
		"webhook_subscription": systemtext.Copy("noun.webhook_subscription", textlang.German),
	},
	textlang.Vietnamese: {
		"activity":             systemtext.Copy("noun.activity", textlang.Vietnamese),
		"company":              systemtext.Copy("noun.company", textlang.Vietnamese),
		"contact":              systemtext.Copy("noun.contact", textlang.Vietnamese),
		"custom_field":         systemtext.Copy("noun.custom_field", textlang.Vietnamese),
		"deal":                 systemtext.Copy("noun.deal", textlang.Vietnamese),
		"deal_room":            systemtext.Copy("noun.deal_room", textlang.Vietnamese),
		"deal_room_comment":    systemtext.Copy("noun.deal_room_comment", textlang.Vietnamese),
		"deal_room_thread":     systemtext.Copy("noun.deal_room_thread", textlang.Vietnamese),
		"lead":                 systemtext.Copy("noun.lead", textlang.Vietnamese),
		"offer":                systemtext.Copy("noun.offer", textlang.Vietnamese),
		"offer_template":       systemtext.Copy("noun.offer_template", textlang.Vietnamese),
		"product":              systemtext.Copy("noun.product", textlang.Vietnamese),
		"project":              systemtext.Copy("noun.project", textlang.Vietnamese),
		"relationship":         systemtext.Copy("noun.relationship", textlang.Vietnamese),
		"saved_view":           systemtext.Copy("noun.saved_view", textlang.Vietnamese),
		"tag":                  systemtext.Copy("noun.tag", textlang.Vietnamese),
		"webhook_subscription": systemtext.Copy("noun.webhook_subscription", textlang.Vietnamese),
	},
}

// RecordNoun is the record type as a reader of lang says it. English reads the
// wire type aloud ("deal room"); a type the language has no word for reads as
// the wire type itself, which is still a true name for it.
func RecordNoun(lang textlang.Lang, recordType string) string {
	nouns, translated := recordNounsByLang[lang]
	if !translated {
		return strings.ReplaceAll(recordType, "_", " ")
	}
	if noun, ok := nouns[recordType]; ok {
		return noun
	}
	return recordType
}

// MoreFields stands for the field names a summary leaves out past its limit;
// %d is how many.
func MoreFields(lang textlang.Lang) string {
	return summaryFor(lang).moreFields
}

// noun names a record type inside this set's sentences. English sentences
// print the wire type as they always have; the others print the language's
// word. A set carrying no language is the zero value, and reads as English.
func (said summaryCopy) noun(recordType string) string {
	if said.lang == textlang.English || said.lang == textlang.Unknown {
		return recordType
	}
	return RecordNoun(said.lang, recordType)
}

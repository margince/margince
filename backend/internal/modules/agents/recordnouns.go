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

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// recordNounsByLang is the record type as a reader of each language says it,
// keyed by the wire type. English has no entry: it reads the wire type aloud.
//
//nolint:goconst // the keys are the contract's wire types, one row per language; a constant per type would be a second spelling of them
var recordNounsByLang = map[textlang.Lang]map[string]string{
	textlang.German: {
		"activity":             "Aktivität",
		"company":              "Unternehmen",
		"contact":              "Kontakt",
		"custom_field":         "Benutzerdefiniertes Feld",
		"deal":                 "Deal",
		"deal_room":            "Deal-Room",
		"deal_room_comment":    "Deal-Room-Kommentar",
		"deal_room_thread":     "Deal-Room-Thread",
		"lead":                 "Lead",
		"offer":                "Angebot",
		"offer_template":       "Angebotsvorlage",
		"product":              "Produkt",
		"project":              "Projekt",
		"relationship":         "Beziehung",
		"saved_view":           "Gespeicherte Ansicht",
		"tag":                  "Schlagwort",
		"webhook_subscription": "Webhook-Abonnement",
	},
	textlang.Vietnamese: {
		"activity":             "hoạt động",
		"company":              "công ty",
		"contact":              "liên hệ",
		"custom_field":         "trường tùy chỉnh",
		"deal":                 "deal",
		"deal_room":            "phòng deal",
		"deal_room_comment":    "bình luận phòng deal",
		"deal_room_thread":     "chủ đề phòng deal",
		"lead":                 "lead",
		"offer":                "báo giá",
		"offer_template":       "mẫu báo giá",
		"product":              "sản phẩm",
		"project":              "dự án",
		"relationship":         "mối quan hệ",
		"saved_view":           "chế độ xem đã lưu",
		"tag":                  "thẻ",
		"webhook_subscription": "đăng ký webhook",
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
// word.
func (said summaryCopy) noun(recordType string) string {
	if said.lang == textlang.English {
		return recordType
	}
	return RecordNoun(said.lang, recordType)
}

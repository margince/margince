// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var nounDefinitions = map[string]Definition{
	"noun.activity":             {Words: langcopy.Phrase{En: "activity", De: "Aktivität", Vi: "hoạt động"}},
	"noun.company":              {Words: langcopy.Phrase{En: "company", De: "Unternehmen", Vi: "công ty"}},
	"noun.contact":              {Words: langcopy.Phrase{En: "contact", De: "Kontakt", Vi: "liên hệ"}},
	"noun.custom_field":         {Words: langcopy.Phrase{En: "custom_field", De: "Benutzerdefiniertes Feld", Vi: "trường tùy chỉnh"}},
	"noun.deal":                 {Words: langcopy.Phrase{En: "deal", De: dealCaption, Vi: "deal"}},
	"noun.deal_room":            {Words: langcopy.Phrase{En: "deal_room", De: "Deal Room", Vi: "phòng deal"}},
	"noun.deal_room_comment":    {Words: langcopy.Phrase{En: "deal_room_comment", De: "Deal-Room-Kommentar", Vi: "bình luận phòng deal"}},
	"noun.deal_room_thread":     {Words: langcopy.Phrase{En: "deal_room_thread", De: "Deal-Room-Thread", Vi: "chủ đề phòng deal"}},
	"noun.lead":                 {Words: langcopy.Phrase{En: "lead", De: "Lead", Vi: "lead"}},
	"noun.offer":                {Words: langcopy.Phrase{En: "offer", De: "Angebot", Vi: "báo giá"}},
	"noun.offer_template":       {Words: langcopy.Phrase{En: "offer_template", De: "Angebotsvorlage", Vi: "mẫu báo giá"}},
	"noun.product":              {Words: langcopy.Phrase{En: "product", De: "Produkt", Vi: "sản phẩm"}},
	"noun.project":              {Words: langcopy.Phrase{En: "project", De: "Projekt", Vi: "dự án"}},
	"noun.relationship":         {Words: langcopy.Phrase{En: "relationship", De: "Beziehung", Vi: "mối quan hệ"}},
	"noun.saved_view":           {Words: langcopy.Phrase{En: "saved_view", De: "Gespeicherte Ansicht", Vi: "chế độ xem đã lưu"}},
	"noun.tag":                  {Words: langcopy.Phrase{En: "tag", De: "Tag", Vi: "thẻ"}},
	"noun.webhook_subscription": {Words: langcopy.Phrase{En: "webhook_subscription", De: "Webhook-Abonnement", Vi: "đăng ký webhook"}},
}

func publicNounDefinitions() map[string]Definition {
	out := map[string]Definition{}
	for code, definition := range nounDefinitions {
		definition.Words.En = strings.ReplaceAll(definition.Words.En, "_", " ")
		out["public_"+code] = definition
	}
	return out
}

// NounArgument localizes known record nouns and preserves extension values.
func NounArgument(recordType string) Argument {
	code := "noun." + recordType
	if _, known := definitionOf(code); known {
		return MessageArgument(code)
	}
	return Text(recordType)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrole

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var roleWords = map[string]langcopy.Phrase{
	Champion:            {En: "champion", De: "Fürsprecher", Vi: "người ủng hộ"},
	EconomicBuyer:       {En: "economic buyer", De: "Budgetverantwortliche Person", Vi: "người quyết định ngân sách"},
	LegacyDecisionMaker: {En: "decision-maker", De: "Entscheidungsträger", Vi: "người ra quyết định"},
	Influencer:          {En: "influencer", De: "Einflussperson", Vi: "người có ảnh hưởng"},
	Blocker:             {En: "blocker", De: "Gegenspieler", Vi: "người cản trở"},
	User:                {En: "user", De: "Nutzer", Vi: "người dùng"},
}

// Recorded is the stored role as its vocabulary key, or "" when it is not one.
// The stored role is free text, so case and outer spaces are not part of it.
func Recorded(role string) string {
	key := strings.ToLower(strings.TrimSpace(role))
	if _, ok := roleWords[key]; !ok {
		return ""
	}
	return key
}

// Label is a role in the reader's language, or "" for a role outside the
// vocabulary: a typed value such as "contact" has no meaning to translate.
func Label(role, language string) string {
	phrase, ok := roleWords[Recorded(role)]
	if !ok {
		return ""
	}
	return langcopy.For(language).Say(phrase)
}

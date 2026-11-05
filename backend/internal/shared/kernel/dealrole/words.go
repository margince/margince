// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrole

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var roleWords = map[string]langcopy.Phrase{
	Champion:            {En: "champion", De: "Fürsprecher", Vi: "người ủng hộ"},
	EconomicBuyer:       {En: "economic buyer", De: "Budgetverantwortliche Person", Vi: "người quyết định ngân sách"},
	LegacyDecisionMaker: {En: "decision-maker", De: "Entscheidungsträger", Vi: "người ra quyết định"},
	Influencer:          {En: "influencer", De: "Einflussperson", Vi: "người có ảnh hưởng"},
	Blocker:             {En: "blocker", De: "Gegenspieler", Vi: "người cản trở"},
	User:                {En: "user", De: "Nutzer", Vi: "người dùng"},
}

// Word labels known roles and preserves an unfamiliar stored role verbatim.
func Word(role, language string) string {
	phrase, ok := roleWords[role]
	if !ok {
		return role
	}
	return langcopy.For(language).Say(phrase)
}

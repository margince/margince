// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
)

// stampHandbookSections names, on each claim from the shipped handbook, the
// section its quote sits in and the settings page that section is about.
//
// The page is read from this binary's own copy. The shipped set was filed from
// that copy, so a claim's lines and the sections around them agree. A claim with
// no line, or on a page this build does not ship, is left as it is.
func stampHandbookSections(claims []crmcontracts.KnowledgeClaim) {
	for i := range claims {
		claim := &claims[i]
		if claim.Line == nil {
			continue
		}
		section, ok := handbook.SectionFor(claim.DocumentName, *claim.Line, lastLine(*claim))
		if !ok {
			continue
		}
		claim.Section = &crmcontracts.KnowledgeClaimSection{Title: section.Title, Line: section.Line}
		if section.Opens != "" {
			claim.Opens = &crmcontracts.KnowledgeClaimOpens{SettingsPage: section.Opens}
		}
	}
}

// lastLine is the line a claim's quote ends on. A bare passage carries no end,
// and its quote is the passage itself, so its lines are counted.
func lastLine(claim crmcontracts.KnowledgeClaim) int {
	if claim.EndLine != nil {
		return *claim.EndLine
	}
	return *claim.Line + strings.Count(strings.TrimRight(claim.Quote, "\n"), "\n")
}

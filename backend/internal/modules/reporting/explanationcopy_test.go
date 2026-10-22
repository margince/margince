// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestComparisonExplainsRefusalInTheSelectedEditionLanguage(t *testing.T) {
	langcopytest.Census(t, comparisonWords)
	langcopytest.Census(t, statusWords)
	for _, tc := range []struct{ language, restricted, population string }{
		{"en", "Historical evidence is restricted or redacted", "Team membership or population changed"},
		{"de", "Historische Belege sind zugriffsbeschränkt oder geschwärzt", "Die Teammitglieder oder die ausgewertete Gruppe haben sich geändert"},
		{"vi", "Bằng chứng lịch sử bị hạn chế truy cập hoặc đã được che", "Thành viên nhóm hoặc tập đối tượng đã thay đổi"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			left := crmcontracts.ReportingEdition{Withheld: true}
			right := crmcontracts.ReportingEdition{Evaluation: crmcontracts.ReportingEvaluation{Context: crmcontracts.ReportingContext{ProseLanguage: &tc.language, PopulationFingerprint: "changed"}}}
			if got := comparisonReason(left, right); got != tc.restricted {
				t.Fatalf("restriction: %q", got)
			}
			left.Withheld = false
			if got := comparisonReason(left, right); got != tc.population {
				t.Fatalf("population: %q", got)
			}
			right.Evaluation.Context.ProseLanguage = nil
			if got := comparisonReason(left, right); got != "Team membership or population changed" {
				t.Fatalf("legacy language: %q", got)
			}
		})
	}
}

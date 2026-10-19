// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestDomainEvidenceHasEveryFieldInEveryShippedLanguage(t *testing.T) {
	langcopytest.Census(t, domainEvidence)
}

func TestDomainEvidencePreservesSourceWordsAndUnknownKinds(t *testing.T) {
	for _, lang := range textlang.Shipped {
		said := domainEvidenceCopyFor(lang)
		quote := "Original evidence: Пример, unchanged."
		got := said.triageEvidence(siteTriageVerdict{Kind: siteKindParked, Reason: quote})
		if !strings.HasSuffix(got, quote) || !strings.Contains(got, said.Say(domainEvidence.Parked)) {
			t.Fatalf("%s changed evidence or lost its kind: %s", lang, got)
		}
		unknown := said.triageEvidence(siteTriageVerdict{Kind: "future_kind"})
		if !strings.Contains(unknown, "future_kind") {
			t.Fatalf("%s invented a known classification", lang)
		}
	}
	if domainEvidenceCopyFor(textlang.Lang("unknown")) != domainEvidenceCopyFor(textlang.English) {
		t.Fatal("unknown language did not use the English fallback")
	}
}

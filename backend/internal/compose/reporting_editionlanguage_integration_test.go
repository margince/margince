// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An edition is stored for every reader, so its captions take the installation's
// language. The owner's own live read takes the owner's language.
func TestAnEditionIsWrittenInTheBaseLanguageWhileTheOwnersLiveReadIsInTheirs(t *testing.T) {
	q := newQuestionEditions(t)
	owner, _ := principal.Actor(q.human)
	owner.Locale = "de"
	q.human = principal.WithActor(q.human, owner)

	edition := q.send(t)
	if got := edition.Evaluation.Context.ProseLanguage; got == nil || *got != "en" {
		t.Fatalf("the stored edition is written in %v, want the base language en", proseLanguageName(got))
	}

	live, err := q.service.Evaluate(q.human, q.report.Selection)
	if err != nil {
		t.Fatal(err)
	}
	if got := live.Context.ProseLanguage; got == nil || *got != "de" {
		t.Fatalf("the owner's live read is written in %v, want de", proseLanguageName(got))
	}
}

// An edition captured in the base language is downloaded by a German reader.
// The file names each metric as the catalog on that reader's screen does.
func TestAnEditionExportNamesItsMetricsAsTheReadersScreenDoes(t *testing.T) {
	q := newQuestionEditions(t)
	edition := q.send(t)
	reader, _ := principal.Actor(q.human)
	reader.Locale = "de"
	german := principal.WithActor(q.human, reader)
	catalog, err := q.service.Catalog(german)
	if err != nil {
		t.Fatal(err)
	}
	shown := map[string]string{}
	for _, metric := range catalog.Metrics {
		shown[string(metric.Id)] = metric.Label
	}
	headlines := exportedEditionRows(german, t, q, edition, "headline")
	if captured := proseLanguageName(edition.Evaluation.Context.ProseLanguage); captured == "de" || len(headlines) == 0 {
		t.Fatalf("captured in %s with headline rows %q, want another language and at least one row", captured, headlines)
	}
	for _, row := range headlines {
		if row[2] != shown[row[0]] || row[2] == reportingMetricNames[crmcontracts.ReportingMetricID(row[0])].En {
			t.Errorf("the file names %s %q, the German screen %q", row[0], row[2], shown[row[0]])
		}
	}
}

func proseLanguageName(prose *string) string {
	if prose == nil {
		return "(none)"
	}
	return *prose
}

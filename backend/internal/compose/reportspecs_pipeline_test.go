// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The converted report's vocabulary, which is the whole guard.
//
// Its default grouping does not include currency, so one stage is one row. That
// is only safe while every money measure it offers is ALREADY converted: a
// native one beside them would let a plan ask for a sum of minor units across
// currencies and get a number with no unit — 10,000 EUR + 10,000 USD = 20,000
// of nothing, where the answer is 15,000 EUR.
//
// The engine refuses a measure a spec does not declare, so keeping the native
// names out of the map IS the refusal. This states it, because the map is a
// place somebody adds a field to without meeting the reason it was absent.

import "testing"

func TestPipelineCurrentOffersNoNativeMoneyMeasure(t *testing.T) {
	spec := pipelineCurrentSpec()

	for _, native := range []string{fieldAmountMinor, fieldWeightedAmountMinor} {
		if _, offered := spec.measures[native]; offered {
			t.Errorf("pipeline-current offers %q.\n\n"+
				"Its rows are not grouped by currency, so summing a native minor-unit "+
				"column across them produces a figure with no denomination. The native "+
				"measures live on deals-by-stage, which groups by currency.", native)
		}
	}

	// And the converted ones ARE there: a guard that passed by the spec having
	// no measures at all would be no guard.
	for _, base := range []string{fieldAmountBaseMinor, fieldWeightedBaseMinor} {
		if _, offered := spec.measures[base]; !offered {
			t.Errorf("pipeline-current does not offer %q, so it can answer no money question", base)
		}
	}
}

// The population is what makes it a COMPOSITION rather than a history: a won
// deal is money that arrived and a lost one is money that did not.
func TestPipelineCurrentMeasuresOnlyOpenDeals(t *testing.T) {
	if got := pipelineCurrentSpec().baseWhere; got != whereOpenDeal {
		t.Errorf("baseWhere = %q, want the shared open-deal population %q", got, whereOpenDeal)
	}
}

// The forecast prices a deal exactly as pipeline-current does.
//
// The two answer neighbouring questions off one pipeline — what it is composed
// of, and what it is expected to bring in — and a reader moves between them on
// one screen. Priced by two expressions, they could disagree about one deal
// while each looked internally consistent, and nothing would say which was
// wrong.
//
// Comparing the rendered SQL rather than asserting both call one helper: the
// helper could be called with different arguments (a different alias, a
// different as-of token) and produce two different expressions from one
// spelling.
func TestTheForecastPricesADealLikeThePipelineDoes(t *testing.T) {
	forecast := prebuiltReports["forecast"]
	pipeline := pipelineCurrentSpec()

	for _, measure := range []string{fieldAmountBaseMinor, fieldWeightedBaseMinor} {
		got, offered := forecast.measures[measure]
		if !offered {
			t.Errorf("the forecast offers no %q, so its categories can only be read "+
				"one currency at a time", measure)
			continue
		}
		if want := pipeline.measures[measure]; got != want {
			t.Errorf("the forecast's %q is not pipeline-current's.\n\n"+
				"forecast:  %s\npipeline:  %s\n\n"+
				"Two spellings of one deal's value can disagree about that deal while "+
				"each stays internally consistent, and the screen showing both would "+
				"give no sign which is wrong.", measure, got, want)
		}
	}

	// The native pair stays. Unlike pipeline-current, this spec's default
	// grouping includes currency, so a native sum under one currency row is a
	// well-defined figure — and deals-by-stage asks the same of it.
	for _, native := range []string{fieldAmountMinor, fieldWeightedAmountMinor} {
		if _, offered := forecast.measures[native]; !offered {
			t.Errorf("the forecast stopped offering %q. Its rows ARE grouped by "+
				"currency, so the native figure is well defined here and something "+
				"reads it.", native)
		}
	}
}

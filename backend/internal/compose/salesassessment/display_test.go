// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"strings"
	"testing"
)

func TestAssessmentDisplaysOnlyTheCitedQuantity(t *testing.T) {
	cell := CellIn{ID: "metric:open_pipeline", Value: "7432189.65 EUR", Unit: "EUR", Display: DisplayQuantity(7432189.65, "EUR", "de"), Coverage: "ok"}
	in := Input{Cells: []CellIn{cell}}
	for _, text := range []string{"Pipeline is {{metric:open_pipeline}}.", "Pipeline is 7432189.65 EUR.", "Pipeline is €7432189.65.", "Pipeline is EUR 7432189.65.", "Pipeline is ≈7.432.190 EUR."} {
		reply := `{"paragraphs":[[{"text":"` + text + `","cells":["metric:open_pipeline"]}]]}`
		got, err := Parse(reply, in)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if !strings.Contains(got.Sentences[0].Text, "≈7.432.190 EUR") {
			t.Fatalf("unformatted quantity: %s", got.Sentences[0].Text)
		}
	}
	for _, text := range []string{"Pipeline is 7432190 EUR.", "Pipeline is 7432189.65 USD.", "Pipeline is {{metric:unknown}}."} {
		reply := `{"paragraphs":[[{"text":"` + text + `","cells":["metric:open_pipeline"]}]]}`
		if _, err := Parse(reply, in); err == nil {
			t.Errorf("accepted unsupported quantity %q", text)
		}
	}
}

func TestACountCannotGroundMoney(t *testing.T) {
	in := Input{Cells: []CellIn{{ID: "count", Value: "71 count", Unit: "count", Display: "71", Coverage: "ok"}}}
	for _, text := range []string{"71 EUR", "$71", "USD71", "71 XYZ", "71 hours", "71 percent", "71 count EUR", "71.00 deals"} {
		if _, err := Parse(`{"paragraphs":[[{"text":"There are `+text+`.","cells":["count"]}]]}`, in); err == nil {
			t.Errorf("count grounded %s", text)
		}
	}
}

func TestWholeQuantityKeepsSmallValuesAndSigns(t *testing.T) {
	for _, tt := range []struct {
		value            float64
		unit, lang, want string
	}{
		{0, "EUR", "en", "0 EUR"},
		{-1.5, "EUR", "en", "≈-2 EUR"},
		{0.1, "EUR", "en", "<1 EUR"},
		{-0.1, "EUR", "de", "−(<1) EUR"},
		{1.7, "hours", "en", "1 h 42 min"},
		{12345, "count", "vi", "12.345"},
	} {
		if got := DisplayQuantity(tt.value, tt.unit, tt.lang); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

func TestTypedSmallMoneyAndCompoundDurationRemainGrounded(t *testing.T) {
	for _, cell := range []CellIn{
		{ID: "small", Value: "0.01 EUR", Unit: "EUR", Display: DisplayQuantity(0.01, "EUR", "en"), Coverage: "ok"},
		{ID: "duration", Value: "1.7 hours", Unit: "hours", Display: DisplayQuantity(1.7, "hours", "en"), Coverage: "ok"},
	} {
		for _, text := range []string{"Reading: {{" + cell.ID + "}}.", "Reading: " + cell.Display + "."} {
			reply := `{"paragraphs":[[{"text":"` + text + `","cells":["` + cell.ID + `"]}]]}`
			got, err := Parse(reply, Input{Cells: []CellIn{cell}})
			if err != nil || len(got.Sentences) != 1 || !strings.Contains(got.Sentences[0].Text, cell.Display) {
				t.Fatalf("%s: %+v %v", text, got, err)
			}
		}
	}
}

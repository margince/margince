// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var placeholder = regexp.MustCompile(`\{([a-z_]+)(?::q)?\}`)

func placeholdersOf(template string) []string {
	var names []string
	for _, m := range placeholder.FindAllStringSubmatch(template, -1) {
		if !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	slices.Sort(names)
	return names
}

func TestEveryReasonCodeHasAnEnglishRendering(t *testing.T) {
	codes := ReasonCodes()
	if len(codes) < 40 {
		t.Fatalf("catalog holds %d codes, want the whole census", len(codes))
	}
	for _, code := range codes {
		want := ReasonParamNames(code)
		for _, lang := range textlang.Shipped {
			template := ReasonTemplate(code, lang)
			if strings.TrimSpace(template) == "" {
				t.Errorf("%s has no %s sentence", code, lang)
				continue
			}
			if got := placeholdersOf(template); !slices.Equal(got, want) {
				t.Errorf("%s in %s names %v, want exactly the declared %v", code, lang, got, want)
			}
		}
	}
}

func TestAReasonRendersItsValuesAndFallsBackToEnglish(t *testing.T) {
	why := Why(ReasonLinkNotFound, "ref", ` CO-7 `)
	if got, want := why.In(textlang.English), `No company with “ CO-7 ” in this upload, in an earlier import, or in Margince.`; got != want {
		t.Errorf("english = %q, want %q", got, want)
	}
	if got := why.In(textlang.German); !strings.Contains(got, `CO-7`) {
		t.Errorf("german = %q, want the quoted ref", got)
	}
	if got := why.In("fr"); got != why.English() {
		t.Errorf("an unknown language = %q, want the english sentence", got)
	}
	if got := (Reason{Code: "no_such_code"}).English(); got != "no_such_code" {
		t.Errorf("an unknown code renders %q, want the bare code", got)
	}
}

func TestADroppedLinkCarriesItsOwnCode(t *testing.T) {
	for from, twin := range droppedTwin {
		got := Why(from, "ref", "X", "name", "X").WithoutTheLink()
		if got.Code != twin || !strings.HasSuffix(got.English(), " The row was imported without the link.") {
			t.Errorf("%s dropped = %s %q", from, got.Code, got.English())
		}
	}
	if got := Why(ReasonTagNew).WithoutTheLink(); got.Code != ReasonTagNew {
		t.Errorf("a code with no twin changed to %s", got.Code)
	}
}

func TestAReportRoundTripsWithAndWithoutACode(t *testing.T) {
	coded := SkippedRow{ExternalID: "A-1", Line: 4}.Because(Why(ReasonRowKeyEmpty, "column", "id"))
	var back SkippedRow
	encoded, err := json.Marshal(coded)
	if err != nil || json.Unmarshal(encoded, &back) != nil || !reflect.DeepEqual(back, coded) {
		t.Fatalf("round trip = %+v (%v), want %+v", back, err, coded)
	}
	if back.Why().Code != ReasonRowKeyEmpty || back.Why().Params["column"] != "id" {
		t.Fatalf("reason = %+v, want the code and its params back", back.Why())
	}
	var old SkippedRow
	if err := json.Unmarshal([]byte(`{"external_id":"A-2","reason":"free text"}`), &old); err != nil {
		t.Fatalf("a report stored before codes existed does not decode: %v", err)
	}
	if old.Reason != "free text" || old.Code != "" {
		t.Fatalf("old row = %+v, want its text and no code", old)
	}
}

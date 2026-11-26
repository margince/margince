// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package transcriptsummary

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestAGroundedSummaryIsWrittenOntoTheReading(t *testing.T) {
	facts := &activities.TranscriptReadFacts{}
	raw := json.RawMessage(`{"key_points":[{"text":"Pricing moved with the volumes.","source_lines":[2,3]}],` +
		`"next_steps":[{"text":"Lars sends the revised pricing by Friday.","source_lines":[4]}]}`)

	Onto(context.Background(), quiet, facts, raw, 4)

	if facts.SummaryRefused || facts.Summary == nil {
		t.Fatalf("a summary citing only lines the transcript has must be written, got %+v", facts)
	}
	if got := facts.Summary.NextSteps[0].SourceLines; len(got) != 1 || got[0] != 4 {
		t.Errorf("the next step must keep the line it cites, got %v", got)
	}
}

func TestASummaryThatCitesALineTheTranscriptLacksIsDropped(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"a line past the end", `{"key_points":[{"text":"Pricing.","source_lines":[9]}],"next_steps":[]}`, "lines 1 to 4"},
		{"line zero", `{"key_points":[{"text":"Pricing.","source_lines":[0]}],"next_steps":[]}`, "lines 1 to 4"},
		{"no line", `{"key_points":[{"text":"Pricing.","source_lines":[]}],"next_steps":[]}`, "cites no line"},
		{"a next step past the end", `{"key_points":[{"text":"Pricing.","source_lines":[1]}],` +
			`"next_steps":[{"text":"Send it.","source_lines":[5]}]}`, "lines 1 to 4"},
		{"no key point", `{"key_points":[],"next_steps":[]}`, "no key point"},
		{"an empty point", `{"key_points":[{"text":" ","source_lines":[1]}],"next_steps":[]}`, "empty"},
		{"no summary at all", ``, "no summary"},
		{"a summary of the wrong shape", `{"key_points":"Pricing"}`, "JSON shape"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := &activities.TranscriptReadFacts{Found: 1}
			Onto(context.Background(), quiet, facts, json.RawMessage(tc.raw), 4)
			if facts.Summary != nil || !facts.SummaryRefused {
				t.Fatalf("the summary must be dropped and the reading must say so, got %+v", facts)
			}
			if facts.Found != 1 {
				t.Errorf("dropping the summary must leave the rest of the reading as it was, got %+v", facts)
			}
			if _, fault := Read(json.RawMessage(tc.raw), 4); !strings.Contains(fault, tc.want) {
				t.Errorf("want the refusal to contain %q, got %q", tc.want, fault)
			}
		})
	}
}

func TestASummaryLongerThanAFewPointsIsDropped(t *testing.T) {
	points := make([]activities.TranscriptSummaryPoint, activities.MaxTranscriptSummaryPoints+1)
	for i := range points {
		points[i] = activities.TranscriptSummaryPoint{Text: "A point.", SourceLines: []int{1}}
	}
	raw, err := json.Marshal(activities.TranscriptSummary{KeyPoints: points})
	if err != nil {
		t.Fatal(err)
	}
	if _, fault := Read(raw, 4); !strings.Contains(fault, "at most") {
		t.Errorf("want a summary of %d points refused, got %q", len(points), fault)
	}
}

func TestTheReportShowsAMeetingThatAgreedNothingAsAnEmptyList(t *testing.T) {
	standing := &activities.StandingTranscriptSummary{
		KeyPoints: []activities.TranscriptSummaryPoint{{Text: "Pricing.", SourceLines: []int{1}}},
		ReadID:    ids.NewV7(), GeneratedAt: time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
	}
	report := Report(standing)
	if report == nil || report.NextSteps == nil || len(report.NextSteps) != 0 {
		t.Fatalf("next steps must be an empty list, never null, got %+v", report)
	}
	if report.ReadId.String() != standing.ReadID.String() || len(report.KeyPoints) != 1 {
		t.Errorf("the report must name the reading that wrote the summary and carry its points, got %+v", report)
	}
	if Report(nil) != nil {
		t.Error("a meeting with no summary reports none")
	}
}

func TestTheInstructionNamesTheKeyTheSchemaRequires(t *testing.T) {
	if !strings.Contains(Instruction(), `"`+Key+`"`) {
		t.Errorf("the instruction must name %q, the field the reply carries the summary in", Key)
	}
	raw, err := json.Marshal(Schema())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"key_points", "next_steps", "source_lines"} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("the schema must carry %q, got %s", field, raw)
		}
	}
}

// The limit counts characters, so a German or Vietnamese sentence at the limit
// is kept although it takes more bytes.
func TestASummaryPointIsMeasuredInCharacters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		runes int
		kept  bool
	}{{"at the limit", activities.MaxTranscriptSummaryPointText, true}, {"one past it", activities.MaxTranscriptSummaryPointText + 1, false}} {
		t.Run(tc.name, func(t *testing.T) {
			text := strings.Repeat("ữ", tc.runes)
			raw, err := json.Marshal(activities.TranscriptSummary{KeyPoints: []activities.TranscriptSummaryPoint{{Text: text, SourceLines: []int{1}}}})
			if err != nil {
				t.Fatal(err)
			}
			_, fault := Read(raw, 4)
			if (fault == "") != tc.kept {
				t.Fatalf("a point of %d characters: kept=%t want %t (%q)", tc.runes, fault == "", tc.kept, fault)
			}
			if !tc.kept && !strings.Contains(fault, strconv.Itoa(tc.runes)+" characters") {
				t.Errorf("the refusal must count characters, got %q", fault)
			}
		})
	}
}

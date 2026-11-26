// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package transcriptsummary is the short summary a transcript reading writes
// onto its meeting. It holds the reply's part that carries it, the check it
// must pass, and its shape on the report.
//
// It rides the reading's one model call. A summary that fails the check is
// dropped and the proposals beside it still count, so it never fails the reading.
package transcriptsummary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// Key is the reply's field for the summary. It is not "summary", which each
// proposal already uses for its one sentence.
const Key = "meeting_summary"

// Instruction asks for the summary, after the reading's JSON instruction has
// named Key among the fields of the reply.
func Instruction() string {
	return fmt.Sprintf(`"%s" is a short summary of the meeting for a colleague preparing the next call. `+
		`"key_points" are 1 to %[2]d points the meeting covered. "next_steps" are up to %[2]d things it `+
		`agreed would happen next, and an empty list when it agreed none. Each point is one plain sentence `+
		`of at most %[3]d characters, with "source_lines": the 1 to %[4]d line numbers it was read from. `+
		`Write the summary in the language the transcript is spoken in, whatever the LANGUAGE rule says `+
		`for the other fields. Summarize only what was said, never what should happen to the account.`,
		Key, activities.MaxTranscriptSummaryPoints, activities.MaxTranscriptSummaryPointText,
		activities.MaxTranscriptCitedLines)
}

// Schema is the summary's shape in the reply's response schema.
func Schema() schema.Node {
	point := schema.Object(map[string]schema.Node{
		"text":         schema.String(),
		"source_lines": schema.Array(schema.Integer()),
	}, "text", "source_lines")
	return schema.Object(map[string]schema.Node{
		"key_points": schema.Array(point),
		"next_steps": schema.Array(point),
	}, "key_points", "next_steps")
}

// Read decodes the summary from the reply and checks it against a transcript
// of lineCount lines. A non-empty fault means it may not be written.
func Read(raw json.RawMessage, lineCount int) (activities.TranscriptSummary, string) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return activities.TranscriptSummary{}, "the reply carries no summary"
	}
	var summary activities.TranscriptSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return activities.TranscriptSummary{}, "the summary is not the required JSON shape"
	}
	if msg := summary.Fault(lineCount); msg != "" {
		return activities.TranscriptSummary{}, msg
	}
	return summary, ""
}

// Report is the meeting's summary as the report shows it, or nil for none.
func Report(standing *activities.StandingTranscriptSummary) *crmcontracts.TranscriptSummary {
	if standing == nil {
		return nil
	}
	return &crmcontracts.TranscriptSummary{
		ReadId:      openapi_types.UUID(standing.ReadID),
		GeneratedAt: standing.GeneratedAt,
		KeyPoints:   reportPoints(standing.KeyPoints),
		NextSteps:   reportPoints(standing.NextSteps),
	}
}

// reportPoints is never nil: an empty list is the meeting that agreed nothing.
func reportPoints(points []activities.TranscriptSummaryPoint) []crmcontracts.TranscriptSummaryPoint {
	out := make([]crmcontracts.TranscriptSummaryPoint, 0, len(points))
	for _, point := range points {
		out = append(out, crmcontracts.TranscriptSummaryPoint{Text: point.Text, SourceLines: point.SourceLines})
	}
	return out
}

// Onto writes the summary into a done reading's facts, or marks it refused and
// logs why. The reading's id rides ctx as its correlation id.
func Onto(ctx context.Context, log *slog.Logger, facts *activities.TranscriptReadFacts, raw json.RawMessage, lineCount int) {
	summary, fault := Read(raw, lineCount)
	if fault != "" {
		facts.SummaryRefused = true
		log.WarnContext(ctx, "transcript summary refused", "reason", fault)
		return
	}
	facts.Summary = &summary
}

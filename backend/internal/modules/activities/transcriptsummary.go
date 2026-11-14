// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The short summary a transcript reading writes onto its meeting. It lives in
// the reading's outcome, so the erasure that deletes the readings takes it too.
// The meeting's summary is the newest one any reading wrote.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The bounds a summary is held to. It is a few points a colleague reads before
// the next call, each pointing at its lines.
const (
	MaxTranscriptSummaryPoints    = 5
	MaxTranscriptSummaryPointText = 240
)

// MaxTranscriptCitedLines bounds one claim's citation, a proposal's or a
// summary point's. A claim stated across more lines is not located.
const MaxTranscriptCitedLines = 6

// TranscriptSummary is what a model wrote about one meeting.
type TranscriptSummary struct {
	KeyPoints []TranscriptSummaryPoint `json:"key_points"`
	NextSteps []TranscriptSummaryPoint `json:"next_steps"`
}

// TranscriptSummaryPoint is one sentence and the transcript lines behind it.
type TranscriptSummaryPoint struct {
	Text        string `json:"text"`
	SourceLines []int  `json:"source_lines"`
}

// StandingTranscriptSummary is the meeting's summary and the reading that wrote it.
type StandingTranscriptSummary struct {
	TranscriptSummary
	ReadID      ids.UUID
	GeneratedAt time.Time
}

// TranscriptCitationFault names what is wrong with one claim's cited lines, or
// "" when it cites 1 to most lines, each one the transcript has.
func TranscriptCitationFault(cited []int, lineCount, most int) string {
	switch {
	case len(cited) == 0:
		return "cites no line, and an uncited claim is a guess"
	case len(cited) > most:
		return fmt.Sprintf("cites %d lines, and at most %d may be cited", len(cited), most)
	}
	for _, line := range cited {
		if line < 1 || line > lineCount {
			return fmt.Sprintf("cites line %d, and this transcript has lines 1 to %d", line, lineCount)
		}
	}
	return ""
}

// Fault names the first reason this summary may not be written onto a meeting
// whose transcript has lineCount lines, or "" when it may.
func (s TranscriptSummary) Fault(lineCount int) string {
	if len(s.KeyPoints) == 0 {
		return "the summary states no key point"
	}
	for _, part := range []struct {
		name   string
		points []TranscriptSummaryPoint
	}{{"key points", s.KeyPoints}, {"next steps", s.NextSteps}} {
		if len(part.points) > MaxTranscriptSummaryPoints {
			return fmt.Sprintf("the summary has %d %s, and at most %d may be written",
				len(part.points), part.name, MaxTranscriptSummaryPoints)
		}
		for _, point := range part.points {
			if msg := summaryPointFault(point, lineCount); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func summaryPointFault(point TranscriptSummaryPoint, lineCount int) string {
	switch {
	case strings.TrimSpace(point.Text) == "":
		return "a summary point is empty"
	case utf8.RuneCountInString(point.Text) > MaxTranscriptSummaryPointText:
		return fmt.Sprintf("a summary point is %d characters, and at most %d may be written",
			utf8.RuneCountInString(point.Text), MaxTranscriptSummaryPointText)
	}
	if msg := TranscriptCitationFault(point.SourceLines, lineCount, MaxTranscriptCitedLines); msg != "" {
		return "a summary point " + msg
	}
	return ""
}

// standingTranscriptSummary is the newest summary a reading wrote, up to and
// including read. A later reading that wrote none leaves it standing. found is
// false when no reading wrote one.
func standingTranscriptSummary(ctx context.Context, tx pgx.Tx, read TranscriptRead) (StandingTranscriptSummary, bool, error) {
	var out StandingTranscriptSummary
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT id, finished_at, outcome->'summary'
		  FROM transcript_read
		 WHERE activity_id = $1 AND (created_at, id) <= ($2, $3) AND outcome ? 'summary'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`, read.ActivityID, read.CreatedAt, read.ID).Scan(&out.ReadID, &out.GeneratedAt, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return StandingTranscriptSummary{}, false, nil
	}
	if err != nil {
		return StandingTranscriptSummary{}, false, fmt.Errorf("activities: reading a meeting's transcript summary: %w", err)
	}
	if err := json.Unmarshal(raw, &out.TranscriptSummary); err != nil {
		return StandingTranscriptSummary{}, false, fmt.Errorf("activities: decoding a meeting's transcript summary: %w", err)
	}
	return out, true, nil
}

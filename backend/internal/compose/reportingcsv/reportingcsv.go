// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package reportingcsv holds the evaluation-free parts of the reporting CSV
// export: figure labels, amounts in major units and the file name.
package reportingcsv

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/formulacell"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Labels name the figures of a target comparison, a forecast
// basis and a chart's ends the way the panels do. The key stays the machine id
// in the row's context.
var Labels = map[string]langcopy.Phrase{
	"status":             {En: "Status", De: "Status", Vi: "Trạng thái"},
	"target":             {En: "Goal", De: "Ziel", Vi: "Mục tiêu"},
	"landing":            {En: "Expected landing", De: "Erwartetes Ergebnis", Vi: "Kết quả dự kiến"},
	"won":                {En: "Won", De: "Gewonnen", Vi: "Đã thắng"},
	"gap":                {En: "Gap to goal", De: "Abstand zum Ziel", Vi: "Chênh lệch so với mục tiêu"},
	"remaining":          {En: "Left to win", De: "Noch zu gewinnen", Vi: "Còn phải thắng"},
	"open_pipeline":      {En: "Open pipeline", De: "Offene Pipeline", Vi: "Pipeline đang mở"},
	"coverage":           {En: "Coverage, deals closing this quarter", De: "Abdeckung, Deals mit Abschluss in diesem Quartal", Vi: "Mức bao phủ, deal chốt trong quý này"},
	"measure":            {En: "Forecast measure", De: "Prognosegrundlage", Vi: "Cơ sở dự báo"},
	"records_projection": {En: "Projection from records", De: "Hochrechnung aus den Datensätzen", Vi: "Dự báo từ dữ liệu"},
	"manager_call":       {En: "Manager's call", De: "Einschätzung der Führungskraft", Vi: "Nhận định của quản lý"},
	"caveat":             {En: "Caveat", De: "Einschränkung", Vi: "Lưu ý"},
	"range_low":          {En: "Low end of 8 in 10 outcomes", De: "Untere Grenze von 8 aus 10 Ergebnissen", Vi: "Mức thấp của 8 trên 10 kết quả"},
	"range_high":         {En: "High end of 8 in 10 outcomes", De: "Obere Grenze von 8 aus 10 Ergebnissen", Vi: "Mức cao của 8 trên 10 kết quả"},
	"opening":            {En: "Opening", De: "Anfang", Vi: "Đầu kỳ"},
	"closing":            {En: "Closing", De: "Ende", Vi: "Cuối kỳ"},
	"assessment":         {En: "Assessment", De: "Einschätzung", Vi: "Đánh giá"},
}

// Major writes an amount of minor units as major units, with the digits the
// currency has. A figure outside int64 keeps its plain number rather than wrapping.
func Major(minor *float64, currency string) string {
	if minor == nil {
		return ""
	}
	if math.IsNaN(*minor) || math.Abs(*minor) >= math.MaxInt64 {
		return strconv.FormatFloat(*minor, 'f', -1, 64)
	}
	return values.MajorUnits(int64(math.Round(*minor)), currency)
}

// Filename names the file for its scope, period and the evaluation's
// day in the workspace zone, so exports of different questions never overwrite
// each other.
func Filename(frame crmcontracts.ReportingContext) string {
	zone, err := time.LoadLocation(frame.Timezone)
	if err != nil {
		zone = time.UTC
	}
	scope := string(frame.Scope.Kind)
	if frame.Scope.Label != nil && *frame.Scope.Label != "" {
		scope = *frame.Scope.Label
	}
	parts := []string{"reporting", fileNameWord(scope), fileNameWord(frame.PeriodKind), frame.EvaluatedAt.In(zone).Format(time.DateOnly)}
	kept := parts[:0]
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "-") + ".csv"
}

// fileNameWord keeps letters and digits of a name, so it holds no path
// separator, quote or space.
func fileNameWord(name string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			dash = false
		} else if !dash && out.Len() > 0 {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(out.String(), "-")
}

// IDOf is the figure id whose English label is the given text, or the text
// itself when none matches. A reader of the file in English uses it to find a row.
func IDOf(english string) string {
	for id, phrase := range Labels {
		if phrase.En == english {
			return id
		}
	}
	return english
}

// AnswerCell writes one measure of an analytics answer, money in major units.
//
//craft:ignore naked-any an answer row's values are schemaless JSON, decoded as any
func AnswerCell(value any, money bool, currency string) string {
	switch number := value.(type) {
	case nil:
		return ""
	case float64:
		if money {
			return Major(&number, currency)
		}
		return strconv.FormatFloat(number, 'f', -1, 64)
	default:
		return formulacell.Guard(fmt.Sprint(number))
	}
}

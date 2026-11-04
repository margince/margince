// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	unitHours   = "hours"
	unitDays    = "days"
	unitPercent = "percent"
)

var (
	moneyQuantity = regexp.MustCompile(`(?:≈)?(?:[A-Z]{3}\s*[-+]?\d[\d.,]*(?:\s?[kKmMbB]\b)?|[-+]?\d[\d.,]*(?:\s?[kKmMbB]\b)?\s*[A-Z]{3}\b)`)
	currencyCode  = regexp.MustCompile(`[A-Z]{3}`)
	factToken     = regexp.MustCompile(`\{\{([^{}]+)\}\}`)
)

// DisplayQuantity keeps small values distinct from zero when interpreted figures are rounded.
func DisplayQuantity(value float64, unit, language string) string {
	if value == 0 && unit != unitHours {
		return "0" + displayUnit(unit, language)
	}
	if unit == unitHours {
		return displayHours(value)
	}
	rounded := math.Round(value)
	prefix := ""
	if value != rounded {
		prefix = "≈"
	}
	if value != 0 && rounded == 0 {
		if value < 0 {
			return "−(<1)" + displayUnit(unit, language)
		}
		return "<1" + displayUnit(unit, language)
	}
	text := strconv.FormatFloat(rounded, 'f', 0, 64)
	separator := ","
	if language == "de" || language == "vi" {
		separator = "."
	}
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign = "-"
		text = text[1:]
	}
	for at := len(text) - 3; at > 0; at -= 3 {
		text = text[:at] + separator + text[at:]
	}
	return prefix + sign + text + displayUnit(unit, language)
}

func displayHours(value float64) string {
	minutes := int64(math.Round(math.Abs(value) * 60))
	if value != 0 && minutes == 0 {
		if value < 0 {
			return "−<1 min"
		}
		return "<1 min"
	}
	text := strconv.FormatInt(minutes/60, 10) + " h " + strconv.FormatInt(minutes%60, 10) + " min"
	if value < 0 {
		return "−" + text
	}
	return text
}

func displayUnit(unit, language string) string {
	if unit == "count" || unit == "ratio" || unit == "" {
		return ""
	}
	if unit == unitDays {
		switch language {
		case "de":
			return " Tage"
		case "vi":
			return " ngày"
		}
	}
	if unit == unitPercent {
		return "%"
	}
	return " " + unit
}

// CellDisplay keeps older callers readable when they supply no display alias.
func CellDisplay(cell CellIn) string {
	if cell.Display != "" {
		return cell.Display
	}
	return cell.Value
}

// Rendering uses only cited facts; their formatted aliases cannot ground a different quantity.
func renderQuantities(text string, cited []string, cells map[string]CellIn) (string, string, bool) {
	raw, shown := text, text
	for _, token := range factToken.FindAllStringSubmatch(text, -1) {
		cell, ok := cells[token[1]]
		if !ok || !slices.Contains(cited, token[1]) {
			return "", "", false
		}
		raw = strings.ReplaceAll(raw, token[0], cell.Value)
		shown = strings.ReplaceAll(shown, token[0], CellDisplay(cell))
	}
	for _, id := range cited {
		cell, ok := cells[id]
		if !ok || cell.Display == "" {
			continue
		}
		if slices.Contains([]string{unitHours, unitDays, unitPercent}, cell.Unit) || values.ValidCurrency(cell.Unit) {
			raw = replaceQuantity(raw, cell.Display, cell.Value)
		}
		shown = replaceQuantity(shown, cell.Value, cell.Display)
	}
	for _, token := range moneyQuantity.FindAllString(raw, -1) {
		unit := currencyCode.FindString(token)
		if !values.ValidCurrency(unit) {
			return "", "", false
		}
		cell, ok := citedMoney(token, unit, cited, cells)
		if !ok {
			return "", "", false
		}
		shown = replaceQuantity(shown, strings.TrimRight(token, ".,"), CellDisplay(cell))
	}
	shown, valid := renderSymbolAmounts(raw, shown, cited, cells)
	if !valid {
		return "", "", false
	}
	shown, valid = renderUnitAmounts(raw, shown, cited, cells)
	if !valid {
		return "", "", false
	}
	if !validDisplayedQuantities(shown, cited, cells) {
		return "", "", false
	}
	return raw, shown, true
}

func citedMoney(token, unit string, cited []string, cells map[string]CellIn) (CellIn, bool) {
	number := strings.TrimRight(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(token, "≈"), unit), unit)), ".,")
	for _, id := range cited {
		cell, ok := cells[id]
		if !ok || (cell.Unit != unit && (cell.Unit != "" || !strings.HasSuffix(cell.Value, " "+unit))) {
			continue
		}
		if token == CellDisplay(cell) {
			return cell, true
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(cell.Value, unit)), 64)
		if err == nil && quantityCarried(number, []float64{value}) {
			return cell, true
		}
	}
	return CellIn{}, false
}

func machineWording(text string) bool {
	lowered := strings.ToLower(text)
	for _, phrase := range []string{"commit_evidence", "no_target", "qualified_commit_evidence", "i checked", "i verified", "i couldn’t confirm", "i couldn't confirm", "ich habe geprüft", "tôi đã kiểm tra", "nothing checked", "http://", "https://"} {
		if strings.Contains(lowered, phrase) {
			return true
		}
	}
	return false
}

func replaceQuantity(text, from, to string) string {
	if from == "" || from == to {
		return text
	}
	pattern := regexp.MustCompile(`(^|[^\pL\pN.,])` + regexp.QuoteMeta(from) + `($|[^\pL\pN])`)
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		return strings.Replace(match, from, to, 1)
	})
}

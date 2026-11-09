// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

var (
	symbolAmount = regexp.MustCompile(`[-+]?[€$£¥₫]\s*[-+]?\d[\d.,]*(?:\s?[kKmMbB]\b)?|[-+]?\d[\d.,]*(?:\s?[kKmMbB]\b)?\s*[€$£¥₫]`)
	unitAmount   = regexp.MustCompile(`(?i)([-+]?\d[\d.,]*(?:\s?[kKmMbB]\b)?)\s*(count|hours?|hrs?|days?|percent|prozent|%)(?:\b|[.,;!?]|$)`)
)

func renderSymbolAmounts(raw, shown string, cited []string, cells map[string]CellIn) (string, bool) {
	for _, token := range symbolAmount.FindAllString(raw, -1) {
		token = strings.TrimRight(token, ".,")
		number := quantityPattern.FindString(token)
		if strings.HasPrefix(token, "-") && !strings.HasPrefix(number, "-") {
			number = "-" + number
		}
		found := false
		for _, id := range cited {
			cell := cells[id]
			symbol, ok := values.CurrencySymbol(cell.Unit)
			if !ok || !strings.Contains(token, symbol) || !cellCarriesQuantity(cell, number) {
				continue
			}
			shown = replaceQuantity(shown, token, CellDisplay(cell))
			found = true
			break
		}
		if !found {
			return "", false
		}
	}
	return shown, true
}

func renderUnitAmounts(raw, shown string, cited []string, cells map[string]CellIn) (string, bool) {
	for _, match := range unitAmount.FindAllStringSubmatch(raw, -1) {
		unit := quantityUnit(match[2])
		found := false
		for _, id := range cited {
			cell := cells[id]
			if cell.Unit != unit || !cellCarriesQuantity(cell, match[1]) {
				continue
			}
			shown = replaceQuantity(shown, strings.TrimRight(match[0], ".,;!?"), CellDisplay(cell))
			found = true
			break
		}
		if !found {
			return "", false
		}
	}
	return shown, true
}

func cellCarriesQuantity(cell CellIn, number string) bool {
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(cell.Value, cell.Unit)), 64)
	return err == nil && quantityCarried(strings.TrimRight(number, ".,"), []float64{value})
}

func quantityUnit(unit string) string {
	switch strings.ToLower(unit) {
	case "hour", unitHours, "hr", "hrs":
		return unitHours
	case "day", unitDays:
		return unitDays
	case unitPercent, "prozent", "%":
		return unitPercent
	default:
		return "count"
	}
}

var displayFraction = regexp.MustCompile(`[.,]\d{1,2}$`)

func validDisplayedQuantities(shown string, cited []string, cells map[string]CellIn) bool {
	for _, number := range quantityPattern.FindAllString(shown, -1) {
		number = strings.TrimRight(number, ".,")
		if displayFraction.MatchString(number) || strings.ContainsAny(number, "kKmMbB") {
			return false
		}
	}
	raw := shown
	for _, id := range cited {
		cell := cells[id]
		if values.ValidCurrency(cell.Unit) && cell.Display != "" {
			raw = replaceQuantity(raw, cell.Display, cell.Value)
		}
	}
	for _, token := range moneyQuantity.FindAllString(raw, -1) {
		unit := currencyCode.FindString(token)
		if _, ok := citedMoney(token, unit, cited, cells); !ok {
			return false
		}
	}
	_, valid := renderSymbolAmounts(raw, raw, cited, cells)
	return valid
}

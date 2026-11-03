// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	quantityPattern = regexp.MustCompile(`[-+]?\d[\d.,]*(?:\s?[kKmMbB]\b)?`)
	percentPattern  = regexp.MustCompile(`(?i)(%|\bpercent\b|\bprozent\b|phần trăm)`)
)

func numbersCarried(text string, cited []string, cells map[string]CellIn) bool {
	for _, r := range text {
		if unicode.IsDigit(r) && r > '9' {
			return false
		}
	}
	carried := []float64{}
	percent := false
	for _, id := range cited {
		cell := cells[id]
		percent = percent || cell.Unit == "percent"
		for _, token := range quantityPattern.FindAllString(cell.Value, -1) {
			if value, err := strconv.ParseFloat(token, 64); err == nil {
				carried = append(carried, value)
			}
		}
	}
	if percentPattern.MatchString(text) && !percent {
		return false
	}
	for _, token := range quantityPattern.FindAllString(text, -1) {
		if !quantityCarried(token, carried) {
			return false
		}
	}
	return true
}

// A compact suffix must scale to an entire supplied value, never a prefix.
func quantityCarried(token string, carried []float64) bool {
	factor := 1.0
	switch strings.ToLower(token[len(token)-1:]) {
	case "k":
		factor = 1e3
	case "m":
		factor = 1e6
	case "b":
		factor = 1e9
	}
	if factor != 1 {
		token = strings.TrimSpace(token[:len(token)-1])
	}
	variants := []string{token, strings.ReplaceAll(token, ",", ""), strings.ReplaceAll(token, ".", "")}
	if last := strings.LastIndexAny(token, ",."); last >= 0 {
		whole := strings.NewReplacer(",", "", ".", "").Replace(token[:last])
		variants = append(variants, whole+"."+token[last+1:])
	}
	for _, variant := range variants {
		value, err := strconv.ParseFloat(variant, 64)
		if err != nil {
			continue
		}
		for _, source := range carried {
			if math.Abs(value*factor-source) < 1e-9 {
				return true
			}
		}
	}
	return false
}

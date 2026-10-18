// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The size_band column is a closed enum; employee_range is the page's own
// phrasing ("25 to 50", "about 120 contacts", "140+"). A phrase lands in a
// band when one band holds the headcount it states. A floor ("140+", "over
// 500") lands in the band holding its lower bound: a size filter finds the
// company in the smallest honest band, and finds nothing if the column stays
// empty. A ceiling ("up to 50") and a range across bands still refuse.

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// sizeBands mirrors the company.size_band CHECK (0005) in ascending
// order; the top band is open-ended.
var sizeBands = []struct {
	label  string
	lo, hi int
}{
	{"1-10", 1, 10},
	{"11-50", 11, 50},
	{"51-200", 51, 200},
	{"201-500", 201, 500},
	{"501-1000", 501, 1000},
	{"1001-5000", 1001, 5000},
	{"5000+", 5001, math.MaxInt},
}

var (
	// A magnitude word next to a digit ("10k", "2 thousand", "2-million")
	// means the digits alone are NOT the headcount — always a refusal,
	// never a parse.
	employeeRangeMagnitude = regexp.MustCompile(`(?i)\d\s*[-–.]?\s*(k|m|thousand|tausend|tsd|million|mio|mrd|billion)($|[^\p{L}])`)
	// A floor marker states a lower bound. The exclusive ones (">500", "over
	// 500") start one above the number; the inclusive ones ("at least 500")
	// start on it.
	employeeRangeAbove   = regexp.MustCompile(`(?i)>|(^|[^\p{L}])(over|more than|mehr als|über)([^\p{L}]|$)`)
	employeeRangeAtLeast = regexp.MustCompile(`(?i)≥|(^|[^\p{L}])(at least|mindestens)([^\p{L}]|$)`)
	// A ceiling marker ("up to 50") could mean any band below it.
	employeeRangeCeiling = regexp.MustCompile(`(?i)[<≤]|(^|[^\p{L}])(under|less than|fewer than|up to|unter|weniger als|bis zu|höchstens)([^\p{L}]|$)`)
	// A company-register marker means the digits are an identifier, not a
	// headcount ("HRB 9001").
	employeeRangeRegister = regexp.MustCompile(`(?i)(^|[^\p{L}])(hrb|hra|vr|vat|ust)([^\p{L}\d]|\d|$)`)
	employeeRangeNumber   = regexp.MustCompile(`\d[\d,.]*`)
	// Thousands separators are the one punctuation a headcount may carry,
	// and only one KIND of them per number; "2.5" is a decimal and
	// "1,234.567" is mixed — neither is anybody's clean headcount.
	thousandsShaped = regexp.MustCompile(`^\d{1,3}(,\d{3})*$|^\d{1,3}(\.\d{3})*$|^\d+$`)
)

// sizeBandFromEmployeeRange maps a stated headcount phrase onto the
// size_band enum, reporting false whenever the phrasing does not land in
// exactly one band.
func sizeBandFromEmployeeRange(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	for _, band := range sizeBands {
		if trimmed == band.label {
			return band.label, true
		}
	}
	// A leading minus is a parse artifact ("-5"), not a headcount.
	if strings.HasPrefix(trimmed, "-") ||
		employeeRangeMagnitude.MatchString(trimmed) ||
		employeeRangeRegister.MatchString(trimmed) ||
		employeeRangeCeiling.MatchString(trimmed) {
		return "", false
	}
	numbers, ok := employeeRangeNumbers(trimmed)
	if !ok {
		return "", false
	}
	above := employeeRangeAbove.MatchString(trimmed)
	if above || employeeRangeAtLeast.MatchString(trimmed) || strings.Contains(trimmed, "+") {
		// A floor names one number; "50-200+" is no floor anybody can read.
		if len(numbers) != 1 {
			return "", false
		}
		if above {
			return bandContaining(numbers[0] + 1)
		}
		return bandContaining(numbers[0])
	}
	switch len(numbers) {
	case 1:
		return bandContaining(numbers[0])
	case 2:
		return bandHoldingRange(numbers[0], numbers[1])
	}
	return "", false
}

// bandHoldingRange places a stated range when both ends share one band. Sites
// print band edges on round numbers ("50-200", "200-500"), one below the
// enum's own floors, so a range that starts on a band's ceiling is read as
// the band above it.
func bandHoldingRange(lo, hi int) (string, bool) {
	if lo > hi {
		return "", false
	}
	for _, band := range sizeBands {
		if lo == band.hi && hi > lo {
			lo++
			break
		}
	}
	lowBand, okLow := bandContaining(lo)
	highBand, okHigh := bandContaining(hi)
	if okLow && okHigh && lowBand == highBand {
		return lowBand, true
	}
	return "", false
}

// employeeRangeNumbers extracts the headcounts a phrase states, refusing
// any token whose punctuation is not thousands-shaped.
func employeeRangeNumbers(text string) ([]int, bool) {
	tokens := employeeRangeNumber.FindAllString(text, -1)
	numbers := make([]int, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimRight(token, ",.")
		if !thousandsShaped.MatchString(token) {
			return nil, false
		}
		n, err := strconv.Atoi(strings.NewReplacer(",", "", ".", "").Replace(token))
		if err != nil {
			return nil, false
		}
		numbers = append(numbers, n)
	}
	return numbers, len(numbers) > 0
}

func bandContaining(n int) (string, bool) {
	for _, band := range sizeBands {
		if n >= band.lo && n <= band.hi {
			return band.label, true
		}
	}
	return "", false
}

// fillSizeBandFromFacts promotes an accepted employee_range fact onto the
// size_band column — only while the column is empty, so a human's (or an
// import's) standing value is never overwritten, and only when the phrasing
// maps cleanly. On abstention the fact row stays the evidence and the column
// stays fillable by a later, cleaner read.
//
// appliedFacts is upsertCompanyFacts' report of what actually landed: a
// proposal whose employee_range the human-precedence guard refused must not
// promote either, or the column would contradict the standing human fact.
func fillSizeBandFromFacts(ctx context.Context, tx pgx.Tx, in DeepReadProposal, by string, appliedFacts []map[string]any) error {
	landed := false
	for _, applied := range appliedFacts {
		landed = landed || applied["field"] == FactEmployeeRange
	}
	if !landed {
		return nil
	}
	for _, f := range in.Facts {
		if f.Category != factCategoryCompany || f.Field != FactEmployeeRange {
			continue
		}
		band, ok := sizeBandFromEmployeeRange(f.Value)
		if !ok {
			return nil
		}
		// archived_at IS NULL beside size_band IS NULL: the deep-read apply
		// that calls this probes a live company, and the statement says so
		// itself rather than inheriting it, because a promotion onto an archived
		// company is wrong no matter which caller reaches this helper.
		tag, err := tx.Exec(ctx,
			`UPDATE company SET size_band = $2
			  WHERE id = $1 AND size_band IS NULL AND archived_at IS NULL`,
			in.CompanyID, band)
		if err != nil {
			return fmt.Errorf("fill size_band: %w", err)
		}
		if tag.RowsAffected() == 1 {
			confidence := f.Confidence
			stamp := storekit.FieldStamp{Field: "size_band", Confidence: &confidence}
			if f.SourceURL != "" {
				stamp.EvidenceRef = &f.SourceURL
			}
			if err := storekit.StampFields(ctx, tx, "company", in.CompanyID.UUID,
				companySourceSiteRead, by, []storekit.FieldStamp{stamp}); err != nil {
				return err
			}
		}
		// employee_range is single-value: there is at most one such fact.
		return nil
	}
	return nil
}

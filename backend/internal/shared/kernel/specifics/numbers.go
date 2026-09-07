// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package specifics

// The figures a sentence can state: plain counts, percentages, money amounts
// and the currencies they are in.
//
// The separator is the whole difficulty. 1.200 is twelve hundred to a German
// reader and one-point-two to an English one, and the same installation writes
// both — so an ambiguous form yields BOTH readings and matches on either. That
// is deliberately the loose direction: admitting a figure whose other reading
// happened to be in the source is a rare wrong keep, while picking one
// convention would be a wrong DROP every time the reader's language was the
// other one, and a lane that drops true sentences stops being used.

import (
	"regexp"
	"strconv"
	"strings"
)

// figure matches a number with the currency or percent mark that makes it a
// checkable fact, either of which may be absent.
var figure = regexp.MustCompile(`(?i)(` + currencyAlternatives + `)?\s?(\d[\d.,]*\d|\d)\s?(%|percent|prozent|phần trăm)?\s?(` + currencyAlternatives + `)?`)

// currencyMark matches any written currency on its own, because that is how
// both sides actually carry one: a sentence writes "€1.200" or "1.200 Euro",
// and the payload it was written from writes `"currency":"EUR"` in a field with
// no figure beside it. Tying the mark to an adjacent number would find it on
// one side and never the other.
var currencyMark = regexp.MustCompile(`(?i)(` + currencyAlternatives + `)`)

// numbersIn returns the figures and currencies a text carries.
//
// `marked` is the asymmetry that keeps this rule from over-reaching. A SOURCE
// offers every figure it contains; a CLAIM states only its PERCENTAGES.
//
// Everything else a sentence counts is routinely DERIVED — three open deals,
// fourteen days since the last reply, a pipeline worth the sum of its deals —
// and the payload holds the parts rather than the total, so a containment check
// over them would drop true sentences for doing arithmetic correctly. The
// product's own deterministic floor is the proof: it writes the total of every
// open deal into a sentence citing the largest one.
//
// A percentage is never that. Nothing in these payloads is a percentage of
// anything, so a discount or a share in generated prose was read off a record
// or it was invented — which is the whole distinction this package is for.
func numbersIn(text string, marked bool) []Specific {
	var found []Specific
	for _, match := range currencyMark.FindAllStringSubmatch(text, -1) {
		code := currencyCode(match[1])
		if code == "" {
			continue
		}
		found = append(found, Specific{Text: match[1], Keys: []string{"cur:" + code}})
	}
	for _, match := range figure.FindAllStringSubmatch(text, -1) {
		values := numberKeys(match[2])
		if len(values) == 0 {
			continue
		}
		percent := match[3] != ""
		if marked && !percent {
			continue
		}
		// A percentage is not the same fact as the bare number: "40% off" and
		// "40 open deals" share a digit and nothing else, and a check that let
		// one answer the other would admit the invention it is here to catch.
		prefix := "num:"
		if percent {
			prefix = "pct:"
		}
		keys := make([]string, 0, len(values))
		for _, value := range values {
			keys = append(keys, prefix+value)
		}
		found = append(found, Specific{Text: strings.TrimSpace(match[0]), Keys: keys})
	}
	return found
}

// numberKeys renders a written figure as the values it could be, canonical and
// without trailing zeros so 1200, 1200.00 and 1.200,000 collapse to one key.
func numberKeys(raw string) []string {
	digits := strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, raw)
	if digits == "" {
		return nil
	}
	switch {
	case strings.Contains(digits, ".") && strings.Contains(digits, ","):
		// Both present: the LAST one is the decimal point and the other groups,
		// which is true in every locale here.
		return canonical(mixed(digits))
	case strings.ContainsAny(digits, ".,"):
		return canonical(ambiguousSeparator(digits)...)
	default:
		return canonical(digits)
	}
}

// mixed resolves a figure carrying both separators.
func mixed(digits string) string {
	decimalAt := strings.LastIndexAny(digits, ".,")
	whole := strings.Map(dropSeparators, digits[:decimalAt])
	return whole + "." + digits[decimalAt+1:]
}

// ambiguousSeparator returns the readings a single kind of separator has.
//
// One separator with exactly three digits after it and digits before it is the
// genuinely ambiguous case, and both readings are returned. Anything else
// resolves: repeated separators only group, and a group of other than three
// digits is only ever a decimal.
func ambiguousSeparator(digits string) []string {
	at := strings.IndexAny(digits, ".,")
	last := strings.LastIndexAny(digits, ".,")
	tail := digits[last+1:]
	grouped := strings.Map(dropSeparators, digits)
	if at != last {
		return []string{grouped}
	}
	decimal := digits[:at] + "." + tail
	if len(tail) == 3 && at > 0 {
		return []string{grouped, decimal}
	}
	return []string{decimal}
}

func dropSeparators(r rune) rune {
	if r == '.' || r == ',' {
		return -1
	}
	return r
}

// canonical parses each reading and renders it back without trailing zeros, so
// two spellings of one value produce one key.
func canonical(readings ...string) []string {
	keys := make([]string, 0, len(readings))
	for _, reading := range readings {
		value, err := strconv.ParseFloat(reading, 64)
		if err != nil {
			continue
		}
		keys = append(keys, strconv.FormatFloat(value, 'f', -1, 64))
	}
	return keys
}

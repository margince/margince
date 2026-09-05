// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Reading the window length a caller asked for, once.
//
// Three transports carry a period — the readings, the calls history, and the
// agent tool — and each used to spell the mapping as an if against `month`
// falling through to quarter. That shape is wrong in the one direction nothing
// on the page can show: a value it does not recognise answers as a QUARTER
// under the word the caller sent, so a week reads as three months labelled
// "week".
//
// Held by: TestEveryContractPeriodValueMapsToAKind
// (backend/gates/forecastperiodparity_test.go), which derives the wire's values
// from crm.yaml and requires each to map.

// fieldPeriod is the contract's spelling of the window a caller asks for. Named
// because three transports point a refusal at it, and a client matching on the
// field cannot tell a typo from a different field.
const fieldPeriod = "period"

// unknownPeriod is the refusal every transport gives for a window this server
// does not know.
//
// A client matching on the code cannot tell one endpoint's refusal from
// another's, and it should not have to: the same bad word is the same mistake
// wherever it arrives. The message names the windows PeriodKindOf admits, so
// the two move together.
func unknownPeriod() error {
	return &values.ParseError{
		Field: fieldPeriod, Code: "unknown_period",
		Message: "a forecast is read over a quarter, a month or a week",
	}
}

// DayNamed turns an `as_of` DATE into an instant that reads as that same day in
// every zone an installation can be in.
//
// openapi parses a `date` as UTC midnight, which is the day the caller wrote
// down only if the installation sits at or east of UTC. In Los Angeles that
// instant is the previous local day, so a reader asking about Monday the 1st
// was answered about the week that ended the day before.
//
// Midday UTC instead: every IANA zone is within 14 hours of UTC, so noon
// cannot be read as a neighbouring date anywhere. The caller who sent no
// as_of keeps their own instant, which is already the moment they are asking
// about — a date and a clock are different inputs and only one of them needs
// this.
func DayNamed(date time.Time) time.Time {
	day := date.UTC()
	return time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
}

// PeriodKindOf maps one wire value to a window length.
//
// An EMPTY string is the documented default rather than a refusal: a caller who
// sent no period did not choose, which is a different thing from one who chose
// a word this server does not know. ok is false only for the latter.
func PeriodKindOf(asked string) (PeriodKind, bool) {
	switch asked {
	case "":
		return PeriodQuarter, true
	case string(PeriodQuarter):
		return PeriodQuarter, true
	case string(PeriodMonth):
		return PeriodMonth, true
	case string(PeriodWeek):
		return PeriodWeek, true
	default:
		return "", false
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The task due-date box offers days inside the window the server accepts. The
// frontend numbers are a declared mirror of values.DueYearsBack and
// values.DueYearsAhead, and this fails when either side changes alone, or when
// a constant disappears so the gate would read nothing.

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const frontendDueWindow = "../frontend/src/format/calendarday.ts"

func TestTheFrontendDueWindowMatchesTheServerOne(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendDueWindow)
	if err != nil {
		t.Fatalf("reading the frontend calendar module: %v", err)
	}
	for name, want := range map[string]int{
		"DUE_YEARS_BACK":  values.DueYearsBack,
		"DUE_YEARS_AHEAD": values.DueYearsAhead,
	} {
		match := regexp.MustCompile(`export const ` + name + ` = (\d+);`).FindSubmatch(source)
		if match == nil {
			t.Fatalf("%s no longer exports %s as a number literal; this gate reads a shape that is gone", frontendDueWindow, name)
		}
		got, convErr := strconv.Atoi(string(match[1]))
		if convErr != nil || got != want {
			t.Errorf("%s = %s in %s, but the server's window is %d", name, match[1], frontendDueWindow, want)
		}
	}
}

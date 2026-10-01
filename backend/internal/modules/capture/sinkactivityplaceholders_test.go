// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The capture insert's columns, placeholders and arguments agree.
//
import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestTheCaptureInsertCountsItsOwnPlaceholders(t *testing.T) {
	t.Parallel()
	seconds := 9000
	statement, args := capturedActivityInsert(context.Background(), connector.NormalizedRecord{}, ActivityFields{Kind: "meeting", DurationSeconds: &seconds}, birthDecision{})

	listStart := strings.Index(statement, "(")
	listEnd := strings.Index(statement, ")")
	if listStart < 0 || listEnd < listStart {
		t.Fatal("the capture insert's column list is no longer parenthesised; this gate reads between ( and )")
	}
	columns := strings.Count(statement[listStart:listEnd], ",") + 1

	if columns != len(args) {
		t.Fatalf("%d columns, %d arguments", columns, len(args))
	}
	highest := 0
	for _, match := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(statement, -1) {
		n, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			t.Fatalf("unreadable placeholder %q: %v", match[0], convErr)
		}
		if n > highest {
			highest = n
		}
	}
	if columns != highest {
		t.Errorf("the capture insert names %d columns but its highest placeholder is $%d — "+
			"a mismatch fails at execution, on a real capture, as a sync that stops with no test having said so",
			columns, highest)
	}

	// Every number from 1 up is used. A gap means an argument is being read
	// into the wrong column, which no count of the highest one can see.
	used := map[int]bool{}
	for _, match := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(statement, -1) {
		n, _ := strconv.Atoi(match[1])
		used[n] = true
	}
	for n := 1; n <= highest; n++ {
		if !used[n] {
			t.Errorf("the capture insert skips $%d, so every argument after it lands in the wrong column", n)
		}
	}
}

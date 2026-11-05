// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every automatic task source has a sentence in the browser saying what made the task.
//
// Such a task often has no body, so its detail drawer showed a title, a record
// and a date. The browser's TASK_ORIGINS names the sources it can explain.
// storekit.TaskSourceFamilies names the ones the backend writes. A source only
// the backend names shows a task nobody can read. A source only the browser
// names is a dead sentence.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

const frontendTaskOrigins = "../frontend/src/components/taskorigin.ts"

// tsOriginEntry reads one key of the map, bare or quoted, since
// `overnight-reconcile` needs the quotes. An entry the parser cannot see is one
// this gate would agree with.
var tsOriginEntry = regexp.MustCompile(`(?m)^\s*["']?([a-z][a-z_-]*)["']?:\s*"tasks\.origin\.`)

func TestTheFrontendExplainsEveryAutomaticTaskSource(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendTaskOrigins)
	if err != nil {
		t.Fatalf("reading the frontend task origins: %v", err)
	}
	const marker = "TASK_ORIGINS: Readonly<Record<string, MessageKey>> = {"
	start := indexAfter(string(source), marker)
	if start < 0 {
		t.Fatalf("%s no longer declares TASK_ORIGINS as an object literal; this gate is reading a shape that is gone", frontendTaskOrigins)
	}
	end := indexAfter(string(source)[start:], "};")
	if end < 0 {
		t.Fatalf("%s's TASK_ORIGINS literal is unterminated", frontendTaskOrigins)
	}
	table := tsComment.ReplaceAllString(string(source)[start:start+end], " ")
	var inTS []string
	for _, m := range tsOriginEntry.FindAllStringSubmatch(table, -1) {
		inTS = append(inTS, m[1])
	}
	if len(inTS) == 0 {
		t.Fatal("no entries parsed out of TASK_ORIGINS; a gate that reads nothing agrees with everything")
	}

	inGo := storekit.TaskSourceFamilies()
	for _, family := range inGo {
		if !slices.Contains(inTS, family) {
			t.Errorf("%s writes tasks with no human behind them and %s has no sentence for it, so its drawer cannot say what made the task", family, frontendTaskOrigins)
		}
	}
	for _, family := range inTS {
		if !slices.Contains(inGo, family) {
			t.Errorf("%s is explained in %s but storekit.TaskSourceFamilies does not name it; remove the sentence or declare the family", family, frontendTaskOrigins)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// A webhook's record link must open the page the app routes that record to.
// The frontend's ENTITY table owns those routes, so this reads it. It fails
// both ways: a link to a screen the app lacks, and a page the links leave out.
func TestWebhookRecordLinksFollowTheAppRoutes(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "frontend", "src", "lib", "entity.ts"))
	if err != nil {
		t.Fatal(err)
	}
	table := string(src)
	start := strings.Index(table, "export const ENTITY:")
	if start < 0 {
		t.Fatal("entity.ts no longer declares ENTITY")
	}
	table = table[start:]
	end := strings.Index(table, "\n};")
	if end < 0 {
		t.Fatal("entity.ts ENTITY no longer ends with a line holding only \"};\"")
	}
	table = table[:end]
	entry := regexp.MustCompile(`(?m)^\s*(\w+): \{\s*route: \(id\) => \(\{ screen: "(\w+)", id \}\)`)
	frontend := map[string]string{}
	for _, m := range entry.FindAllStringSubmatch(table, -1) {
		frontend[m[1]] = m[2]
	}
	// Every key of ENTITY must have been read, so an entry written another way
	// fails here rather than dropping out of the comparison.
	if keys := regexp.MustCompile(`(?m)^  (\w+): \{`).FindAllString(table, -1); len(keys) == 0 || len(keys) != len(frontend) {
		t.Fatalf("ENTITY has %d entries but %d route(s) were read; the pattern no longer matches every entry",
			len(keys), len(frontend))
	}
	if got := recordRoutes; !reflect.DeepEqual(got, frontend) {
		t.Errorf("webhook record links %v differ from the app's record routes %v", got, frontend)
	}
}

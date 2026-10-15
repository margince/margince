// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/modules/webhooks"
)

// A webhook's record link must open the page the app routes that record to.
// The frontend's ENTITY table owns those routes, so this reads it. It fails
// both ways: a link to a screen the app lacks, and a page the links leave out.
func TestWebhookRecordLinksFollowTheAppRoutes(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("..", "frontend", "src", "lib", "entity.ts"))
	if err != nil {
		t.Fatal(err)
	}
	entry := regexp.MustCompile(`(?m)^\s*(\w+): \{\s*route: \(id\) => \(\{ screen: "(\w+)", id \}\)`)
	frontend := map[string]string{}
	for _, m := range entry.FindAllStringSubmatch(string(src), -1) {
		frontend[m[1]] = m[2]
	}
	if len(frontend) < 5 {
		t.Fatalf("read %d record routes from entity.ts; the pattern no longer matches the file", len(frontend))
	}
	if got := webhooks.RecordRoutes(); !reflect.DeepEqual(got, frontend) {
		t.Errorf("webhook record links %v differ from the app's record routes %v", got, frontend)
	}
}

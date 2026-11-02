// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The inline-image rule of a deal's files is spelled once. The deal's Files
// area and its deal room each spelled it, and the copies drifted. One lacked a
// COALESCE and kept a file with no content type out of a room.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// inlineImageTest is the rule's content-type test, in any case and however the
// SQL around it is laid out.
var inlineImageTest = regexp.MustCompile(`(?i)like\s+'image/`)

func TestTheInlineMailImageRuleIsSpelledOnce(t *testing.T) {
	t.Parallel()
	// A second copy has to write the content-type test again, in Go or in SQL.
	const owner = "internal/platform/database/storekit/dealfiles.go"

	var spellings []string
	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, "_test.go") ||
			!strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".sql") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if inlineImageTest.Match(body) {
			spellings = append(spellings, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module tree: %v", err)
	}
	if len(spellings) != 1 || spellings[0] != owner {
		t.Errorf("the inline-image rule is spelled in %v, want only %s. "+
			"Apply storekit.NotInlineMailImageSQL instead, so the Files area and the deal room keep one answer",
			spellings, owner)
	}
}

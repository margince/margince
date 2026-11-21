// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auditentity

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each upkeep subject is one a module audits under that exact name. A name no
// writer spells keeps nothing out of the unplaced count, and a misspelt one
// lets the real subject back in.
func TestEveryUpkeepTypeIsAnAuditedSubject(t *testing.T) {
	var source strings.Builder
	err := filepath.WalkDir(filepath.Join("..", "..", "..", "modules"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		raw, err := os.ReadFile(path)
		source.Write(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range UpkeepTypes() {
		if !strings.Contains(source.String(), `"`+subject+`"`) {
			t.Errorf("no module names the audit subject %q", subject)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryControllerMailNamesTheEntityThatAnswersForItsRecord: a call into
// confirmSender outside recordentity.go would send a controller mail for the
// default entity, whoever owns the record. The wrappers there name the entity.
func TestEveryControllerMailNamesTheEntityThatAnswersForItsRecord(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "recordentity.go" {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, call := range []string{"confirmSender.QueueConfirmationTx(", "confirmSender.PresentConfirmationTx("} {
			if strings.Contains(string(source), call) {
				t.Errorf("%s calls %s directly; go through queueControllerMailTx or presentControllerMailTx", file, call)
			}
		}
	}
	if scanned < 100 {
		t.Fatalf("scanned %d files; the package has far more, so the scan read the wrong directory", scanned)
	}
}

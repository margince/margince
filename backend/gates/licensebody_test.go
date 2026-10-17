// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

//go:build !integration

package gates

// The BUSL-1.1 text below LICENSE's Parameters block never changes. Covenant 4
// of the license forbids it, so only the Parameters block moves between
// releases. The pin holds every commit to that, not only a tagged one.

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

// buslBodyDigest is the SHA-256 of LICENSE from the rule that closes the
// Parameters block to the end of the file.
const buslBodyDigest = "d3fa0f1011ffd3d85c8b0701ea99f8e9888ef6c94a2705f000ec20d36d5cb5a2"

func TestTheBUSLTextBelowTheParametersNeverChanges(t *testing.T) {
	t.Parallel()
	body := buslBody(t, readRepoFile(t, filepath.Join(repoRoot, "LICENSE")))

	digest := sha256.Sum256([]byte(body))
	if got := hex.EncodeToString(digest[:]); got != buslBodyDigest {
		t.Errorf("LICENSE changed below its Parameters block (digest %s). That text is the fixed "+
			"BUSL-1.1 body and Covenant 4 forbids changing it: undo the change, and put a release's "+
			"own values in the Parameters block (docs/reference/license-release-rule.md).", got)
	}
}

// buslBody returns LICENSE from the rule that closes the Parameters block to the
// end. It fails when the block or its rule is missing, so a reshaped file cannot
// hash an empty slice.
func buslBody(t *testing.T, license string) string {
	t.Helper()
	heading := strings.Index(license, "\nParameters\n")
	if heading < 0 {
		t.Fatal("LICENSE has no Parameters heading, so the gate cannot find where the BUSL body starts")
	}
	rule := strings.Index(license[heading:], "\n-----")
	if rule < 0 {
		t.Fatal("LICENSE's Parameters block has no closing ----- rule, so the BUSL body has no start")
	}
	return license[heading+rule+1:]
}

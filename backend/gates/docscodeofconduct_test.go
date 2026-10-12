// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// CODE_OF_CONDUCT.md is the Contributor Covenant 2.1 as adopted, plus the
// project's enforcement contact. A tree-wide rename once rewrote three of its
// words, and nothing failed. The digest pins the adopted bytes, so any edit
// fails here and has to be made on purpose, with the pin updated beside it.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

const adoptedCodeOfConductSHA256 = "aba2ef77efcf21dff03aaa1fbe79b7b67884767db51b040c28dd732ca71b5bb0"

func TestCodeOfConductIsTheAdoptedText(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(docsTreeRoot, "CODE_OF_CONDUCT.md"))
	if err != nil {
		t.Fatalf("read CODE_OF_CONDUCT.md: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != adoptedCodeOfConductSHA256 {
		t.Errorf("CODE_OF_CONDUCT.md changed (sha256 %s). It is adopted text: compare it with the "+
			"Contributor Covenant 2.1 and undo any rewording; a deliberate change to the enforcement "+
			"section updates adoptedCodeOfConductSHA256 in the same commit.", got)
	}
}

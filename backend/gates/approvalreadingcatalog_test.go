// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestTheApprovalReadingCatalogTracksItsCardSources(t *testing.T) {
	t.Parallel()
	var catalog struct {
		Inputs map[string]string `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(readSource(t, "internal/modules/agents/previewcatalog_gen.json")), &catalog); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"lib/approvalpreview.ts", "lib/approvalpreviewfields.ts", "lib/approvalpreviewmergefields.ts", "i18n/en.ts", "i18n/de.ts", "i18n/vi.ts"} {
		if catalog.Inputs[required] == "" {
			t.Fatalf("catalog misses its source %s", required)
		}
	}
	for source, want := range catalog.Inputs {
		sum := sha256.Sum256([]byte(readSource(t, "../frontend/src/"+source)))
		if hex.EncodeToString(sum[:]) != want {
			t.Errorf("%s changed; run pnpm --dir frontend gen:approval-reading", source)
		}
	}
}

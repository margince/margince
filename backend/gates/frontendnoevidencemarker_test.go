// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The server stores one sentence for a manual signal whose evidence was taken
// away. The browser matches that exact sentence to show a translated label.
// If the two spellings drift, the lead screen shows the raw English sentence in
// every language and nothing else fails.

import (
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

const frontendLeadSignals = "../frontend/src/features/leads/leadsignals.tsx"

var tsNoEvidenceMarker = regexp.MustCompile(`export const MANUAL_SIGNAL_NO_EVIDENCE\s*=\s*"([^"\n]*)";`)

func TestTheFrontendNoEvidenceMarkerMatchesTheServers(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendLeadSignals)
	if err != nil {
		t.Fatalf("reading the lead signals screen: %v", err)
	}
	match := tsNoEvidenceMarker.FindSubmatch(source)
	if match == nil {
		t.Fatalf("%s no longer declares MANUAL_SIGNAL_NO_EVIDENCE as an exported string literal, so this gate reads a shape that is gone", frontendLeadSignals)
	}
	if got := string(match[1]); got != contacts.ManualSignalNoEvidence {
		t.Errorf("the browser matches %q and the server stores %q, so stored signals show untranslated", got, contacts.ManualSignalNoEvidence)
	}
}

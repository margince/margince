// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The confidentiality taxonomy has two readers that must agree on ONE list: the
// JSON schema the model is constrained by, and the validator's rejection
// message. The sender taxonomy beside it shipped with those out of step — two
// kinds reached the prompt while the schema still refused them, so on a
// grammar-constrained local rung the model could never emit either and the
// feature was dead with every test green.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// verdictClearedStatus names the one ledger status that makes a thread readable.
func verdictClearedStatus() string { return capture.VerdictCleared }

func TestTheModelMayAnswerEveryConfidentialityKind(t *testing.T) {
	var shape struct {
		Properties struct {
			Results struct {
				Items struct {
					Properties struct {
						Verdict struct {
							Enum []string `json:"enum"`
						} `json:"verdict"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"results"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(confidentialitySchema(), &shape); err != nil {
		t.Fatalf("decoding the confidentiality schema: %v", err)
	}
	got := make(map[string]bool, len(shape.Properties.Results.Items.Properties.Verdict.Enum))
	for _, kind := range shape.Properties.Results.Items.Properties.Verdict.Enum {
		got[kind] = true
	}
	if len(got) == 0 {
		t.Fatal("the confidentiality schema declares no kind enum — this gate would then pass vacuously")
	}
	for _, kind := range confidentialityKindNames() {
		if !got[kind] {
			t.Errorf("kind %q is in the taxonomy and in the prompt, but the response schema refuses it", kind)
		}
	}
	for kind := range got {
		if _, known := statusForConfidentiality(kind); !known {
			t.Errorf("the schema admits %q, which the taxonomy does not define", kind)
		}
	}
}

func TestExactlyOneConfidentialityKindOpensAThread(t *testing.T) {
	// The whole safety argument rests on this asymmetry: a model that is wrong,
	// unavailable or out of budget fails towards privacy because only one
	// answer can open a thread. A second opening kind would not look like a bug
	// — it would look like a more useful taxonomy — so the count is asserted
	// rather than left to the reader of the map.
	opening := make([]string, 0, 1)
	for kind, status := range confidentialityKinds {
		if status == verdictClearedStatus() {
			opening = append(opening, kind)
		}
	}
	if len(opening) != 1 || opening[0] != confidentialityOrdinary {
		t.Fatalf("kinds that OPEN a thread = %v, want exactly [ordinary] — every other kind must hold, "+
			"because a wrong opening answer publishes correspondence and a wrong hold costs one click", opening)
	}
}

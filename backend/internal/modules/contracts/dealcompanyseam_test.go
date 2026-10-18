// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The refusal a rep meets when they correct a mis-filed deal's company has to
// be an instruction, not a verdict. Refusing is the only outcome the rep finds
// out about while they can still decide what they meant. So the message names
// the blocking agreements, the rule, and the move that clears it.
func TestTheCrossCompanyRefusalTellsARepWhatToDo(t *testing.T) {
	err := error(&DealContractsCrossCompanyError{Named: []string{"MSA-2026", "Pilot addendum"}})

	var blocked *DealContractsCrossCompanyError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v, want DealContractsCrossCompanyError", err)
	}
	for _, named := range blocked.Named {
		if !strings.Contains(blocked.Error(), named) {
			t.Errorf("the message does not name %q, so the rep cannot find it: %q", named, blocked.Error())
		}
	}
	for what, phrase := range map[string]string{
		"the rule":           "must name the same company",
		"detaching":          "detach them from this deal",
		"which edit refused": "before moving it",
	} {
		if !strings.Contains(blocked.Error(), phrase) {
			t.Errorf("the message does not state %s: %q", what, blocked.Error())
		}
	}

	field, code, message := blocked.FieldFault()
	if field != "company_id" {
		t.Errorf("faulted field = %q, want the field the caller sent", field)
	}
	if code == "" || message != blocked.Error() {
		t.Errorf("FieldFault = (%q, %q, %q), want the typed code and the message itself", field, code, message)
	}
	if _, ok := errors.AsType[apperrors.FieldFault](err); !ok {
		t.Error("the refusal does not classify as a field fault, so the transport answers 500 rather than 422")
	}
}

// A cut list still says it was cut. Otherwise a rep who detaches everything
// named and meets the refusal again reads it as the product not noticing.
func TestACutListOfBlockingAgreementsSaysSo(t *testing.T) {
	full := &DealContractsCrossCompanyError{Named: []string{"MSA-2026"}}
	cut := &DealContractsCrossCompanyError{Named: []string{"MSA-2026"}, AndOthers: true}

	if strings.Contains(full.Error(), "and others") {
		t.Errorf("a complete list claims to be cut: %q", full.Error())
	}
	if !strings.Contains(cut.Error(), "and others") {
		t.Errorf("a cut list reads as complete: %q", cut.Error())
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// The refusal tells the user which contracts block, the rule, and what clears
// it, as a 422 on the field they sent.
func TestTheDealCompanyRefusalNamesTheContractsAndWhatToDo(t *testing.T) {
	err := error(&DealContractsCrossCompanyError{Count: 2, Named: []string{"MSA-2026", "Pilot addendum"}})

	fault, ok := errors.AsType[apperrors.FieldFault](err)
	if !ok {
		t.Fatal("the refusal is not a field fault, so the transport answers 500 instead of 422")
	}
	field, code, message := fault.FieldFault()
	if field != "company_id" || code != "deal_contracts_cross_company" {
		t.Errorf("fault = (%q, %q), want the deal's company_id and the typed code", field, code)
	}
	for _, want := range []string{"2 contracts", "MSA-2026", "Pilot addendum", "another company", "Detach them", "deal of the new company"} {
		if !strings.Contains(message, want) {
			t.Errorf("the message lacks %q: %q", want, message)
		}
	}
}

// A long list is cut and says so, and the whole sentence fits the transport's
// ceiling, so no name is cut off by it.
func TestALongListOfBlockingContractsIsCutAndFitsTheFaultText(t *testing.T) {
	long := strings.Repeat("Master services agreement ", 4)
	refusal := &DealContractsCrossCompanyError{Count: 9}
	for range namedInRefusal {
		refusal.Named = append(refusal.Named, textcut.RunesMarked(long, nameRunes, "…"))
	}
	message := refusal.Error()
	if !strings.Contains(message, "and 6 more") {
		t.Errorf("a cut list reads as complete: %q", message)
	}
	if utf8.RuneCountInString(message) > httperr.MaxFaultText {
		t.Errorf("the message is %d runes, over the %d the transport keeps", utf8.RuneCountInString(message), httperr.MaxFaultText)
	}
}

// A caller without contract read gets the count and no names.
func TestTheRefusalWithoutNamesStillCountsAndInstructs(t *testing.T) {
	message := (&DealContractsCrossCompanyError{Count: 1}).Error()
	if message != "1 contract on this deal belongs to another company. "+
		"Detach it, or move it to a deal of the new company, before changing the company." {
		t.Errorf("message = %q", message)
	}
}

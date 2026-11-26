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
	for _, want := range []string{"2 contracts", "MSA-2026", "Pilot addendum", "another company", "Archived contracts", "Ask an administrator"} {
		if !strings.Contains(message, want) {
			t.Errorf("the message lacks %q: %q", want, message)
		}
	}
}

// A long list is cut and says so, and the whole sentence fits the transport's
// ceiling, so no name is cut off by it.
func TestALongListOfBlockingContractsIsCutAndFitsTheFaultText(t *testing.T) {
	for name, long := range map[string]string{
		"ascii":     strings.Repeat("Master services agreement ", 4),
		"multibyte": strings.Repeat("契約書", 40),
		"four-byte": strings.Repeat("𝔘", 60),
	} {
		t.Run(name, func(t *testing.T) {
			refusal := &DealContractsCrossCompanyError{Count: 999}
			for range namedInRefusal {
				refusal.Named = append(refusal.Named, refusalName(long))
			}
			message := refusal.Error()
			if !strings.Contains(message, "and 996 more") {
				t.Errorf("a cut list reads as complete: %q", message)
			}
			if len(message) > httperr.MaxFaultText {
				t.Errorf("the message is %d bytes, over the %d the transport keeps", len(message), httperr.MaxFaultText)
			}
			if !strings.HasSuffix(message, "before changing the company.") {
				t.Errorf("the instructions were cut off: %q", message)
			}
			if !utf8.ValidString(message) {
				t.Errorf("a name was cut inside a character: %q", message)
			}
		})
	}
}

// A caller without contract read gets the count and no names.
func TestTheRefusalWithoutNamesStillCountsAndInstructs(t *testing.T) {
	message := (&DealContractsCrossCompanyError{Count: 1}).Error()
	if message != "1 contract on this deal belongs to another company. "+
		"Archived contracts also count. Ask an administrator to correct the contract links before changing the company." {
		t.Errorf("message = %q", message)
	}
}

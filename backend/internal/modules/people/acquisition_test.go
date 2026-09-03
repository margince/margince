// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// That every creation door records why the contact exists, and that the
// vocabulary keeps facts and conclusions apart.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Every production call to createPerson is a door a contact can arrive
// through, and each must leave an acquisition record. The write lives inside
// createPerson so this is true by construction — this proves the construction,
// because the alternative (each door calling it) is how three doors record it
// and the fourth is found by an auditor.
func TestEveryCreationDoorRecordsWhyTheContactExists(t *testing.T) {
	fset := token.NewFileSet()
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package: %v", err)
	}

	var createPersonCalls, recordsAcquisition int
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		{
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				switch id.Name {
				case "createPerson":
					createPersonCalls++
				case "recordAcquisition":
					recordsAcquisition++
				}
				return true
			})
		}
	}
	if createPersonCalls == 0 {
		t.Fatal("found no createPerson calls — the scan is looking in the wrong place")
	}
	// One writer, reached by every door. More than one would mean a door has
	// started recording this for itself, which is where the copies drift.
	if recordsAcquisition != 1 {
		t.Errorf("recordAcquisition is called %d times, want exactly 1 (inside createPerson) — "+
			"a second caller is a second answer to the same question", recordsAcquisition)
	}
}

// The kinds name what HAPPENED. None of them names a permission, because a
// creation surface that could assert a lawful basis would be deciding the
// question the authorization engine exists to ask.
func TestNoAcquisitionKindIsALawfulBasis(t *testing.T) {
	kinds := []string{
		AcquiredSubjectInitiated, AcquiredCustomerContract, AcquiredRequestedQuoteOrMeeting,
		AcquiredInPersonPermission, AcquiredReferral, AcquiredEventOrForm,
		AcquiredPublicOrBusinessSource, AcquiredPurchasedOrImported, AcquiredUnknownLegacy,
	}
	// The basis vocabulary lives in shared/ports/commsauthz. These are facts;
	// those are conclusions argued from facts, and a value appearing in both
	// would be a creation door granting itself permission.
	bases := map[string]bool{
		"contract": true, "precontract_request": true, "consent": true,
		"legitimate_interests": true, "legal_obligation": true,
		"existing_customer_exception": true, "vital_or_security_interest": true,
		"subject_initiated_correspondence": true, "vn_subject_agreement": true,
	}
	for _, k := range kinds {
		if bases[k] {
			t.Errorf("%q is both an acquisition kind and a lawful basis — a door that records it would be deciding permission", k)
		}
	}
	if len(kinds) != 9 {
		t.Errorf("the vocabulary has %d kinds, want the 9 the table's CHECK admits", len(kinds))
	}
}

// Capture creates a record for two different acts, and only one of them is the
// person initiating contact. Recording the weaker case as the stronger would
// put the vocabulary's strongest claim on a cold prospect's file — which is the
// confusion this table exists to prevent.
func TestOnlyARepliedCounterpartyInitiatedContact(t *testing.T) {
	if got := acquiredFromCapture(true); got != AcquiredSubjectInitiated {
		t.Errorf("a reply is the person writing to us: got %q", got)
	}
	if got := acquiredFromCapture(false); got == AcquiredSubjectInitiated {
		t.Errorf("two outbound threads with no answer is US writing to THEM: got %q, which claims they wrote first", got)
	}
}

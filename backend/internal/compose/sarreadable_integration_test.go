// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The Art. 15 package has to be readable by the person receiving it.
//
// Its sections are built as map[string]any straight from pgx rows, and a uuid
// column arrives as [16]byte — which encoding/json renders as sixteen numbers.
// Nothing is lost, and that is exactly why it survived: the bytes ARE the id,
// so every test asserting the export CONTAINS something still passed while the
// document said `[1,160,94,189,…]` to a human who asked what is held about
// them.
//
// So this walks the encoded document rather than any one section. The defect is
// not a property of the two columns that showed it: the next uuid column added
// to any of the twenty-six sections arrives with it, and only a check over the
// whole package sees that coming.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// byteArrayShaped matches a JSON array of 16 small integers — what a [16]byte
// encodes to, and a shape no legitimate value in this document has.
//
// Matched on the ENCODED text rather than by walking the decoded tree, because
// the encoding is what the subject receives: a check over Go values would be
// asserting about a representation nobody is handed.
var byteArrayShaped = regexp.MustCompile(`\[(?:\d{1,3},){15}\d{1,3}\]`)

// TestTheSubjectAccessPackageIsReadable is the gate the ticket asked for.
func TestTheSubjectAccessPackageIsReadable(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Mara Kessler", nil)
	// An employment, so relationships[] is populated — one of the two sections
	// the defect was found in, and the one carrying a uuid that is not the
	// subject's own id.
	org := e.SeedOrg(t, "Kessler GmbH", nil)
	e.WsExec(t, `
		INSERT INTO relationship (person_id, organization_id, kind, source, captured_by)
		VALUES ($1, $2, 'employment', 'test', 'human:test')`, person, org)

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.PersonKind](person))
	if err != nil {
		t.Fatalf("AssembleSAR → %v", err)
	}
	encoded, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("encoding the package: %v", err)
	}

	// The two sections the defect was measured in must actually be populated,
	// or this walk would pass over a document that holds nothing to judge.
	if len(pkg.Subject) == 0 {
		t.Fatal("the package carries no subject section — this walk would then read a document with " +
			"nothing in it and report the defect gone")
	}
	if len(pkg.Relationships) == 0 {
		t.Fatal("the package carries no relationships — the seed above is what puts a uuid that is NOT " +
			"the subject's own id into a map-shaped section, and without it this proves half the case")
	}

	if found := byteArrayShaped.FindAllString(string(encoded), -1); len(found) > 0 {
		t.Errorf("the package a data subject receives renders %d value(s) as byte arrays rather than as "+
			"something a person can read: %s\n\nA uuid column scans as [16]byte and encoding/json has one "+
			"answer for that. Render it in privacy.readableValue, which every map-shaped section is built "+
			"through", len(found), strings.Join(sample(found), ", "))
	}
}

// sample bounds what a failure prints: the shape repeats, and a document with
// twenty of them is not twenty different findings.
func sample(found []string) []string {
	const most = 3
	if len(found) <= most {
		return found
	}
	return append(found[:most:most], fmt.Sprintf("… and %d more", len(found)-most))
}

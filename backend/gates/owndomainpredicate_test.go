// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// One spelling of "this message came from one of our own domains".
//
// Two readers judge the same messages by it — the waiting queue and the
// response reading — and they must agree: a message the queue hides as internal
// must not count against the answer rate, or /worklist/response reports a
// workspace answering work it never showed anybody.
//
// The predicate is also easy to get subtly wrong in a way that suppresses a
// real customer silently, which is why a second copy is worse here than
// elsewhere. Both known ways were live in the first version of it: reading the
// domain after the FIRST at-sign (a quoted local part may contain one), and
// comparing the suffix with LIKE (an operator's underscore is a wildcard). A
// second copy is a second place for either to come back.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheOwnDomainSenderPredicateHasOneSpelling fails when a second query
// derives a sender's domain for itself instead of calling ownDomainSenderSQL.
func TestTheOwnDomainSenderPredicateHasOneSpelling(t *testing.T) {
	t.Parallel()
	// The shape of deriving a domain from an address in SQL. Any query doing
	// this is answering the question ownDomainSenderSQL already answers.
	const derivesADomain = "split_part(ours.address, '@'"

	var found []string
	root := "internal"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), derivesADomain) {
			return nil
		}
		// The one spelling itself is where it is allowed to appear.
		if strings.HasSuffix(path, filepath.Join("modules", "activities", "waiting.go")) {
			return nil
		}
		found = append(found, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	if len(found) > 0 {
		t.Errorf("these files derive a sender's mail domain in SQL rather than "+
			"calling activities.ownDomainSenderSQL: %v\n\n"+
			"Two spellings of this rule drift, and the drift is invisible: each "+
			"way it goes wrong — splitting at the first at-sign, comparing the "+
			"suffix with LIKE — hides a real customer's mail with nothing on any "+
			"page to say why. Call the shared fragment, or say here why this one "+
			"cannot.", found)
	}
	// The subject must exist, or this walk passes over an empty tree — the
	// under-recognition failure a census must not have.
	subject, err := os.ReadFile(filepath.Join(root, "modules", "activities", "waiting.go"))
	if err != nil {
		t.Fatalf("reading the predicate's own file: %v", err)
	}
	if !strings.Contains(string(subject), derivesADomain) {
		t.Fatal("ownDomainSenderSQL no longer derives a domain the way this gate " +
			"looks for, so the walk above can no longer find a second copy either")
	}
}

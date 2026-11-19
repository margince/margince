// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importkeys

import "testing"

// A domain that does not parse keeps the file's spelling, and the ledger still
// meets it without case, as the store does.
func TestTheLedgerMeetsADomainWithoutCase(t *testing.T) {
	ledger := Ledger{}
	ledger.Claim("company", map[Kind]string{Domain: "Not_A.Domain"})
	if !ledger.Held(Domain, "not_a.domain", "company") {
		t.Fatal("a domain claimed in one case is not held in another")
	}
}

// A lead's LinkedIn URL keeps the case of its path in the store, so the ledger
// keeps it too.
func TestTheLedgerKeepsTheCaseOfALinkedInURL(t *testing.T) {
	ledger := Ledger{}
	ledger.Claim("lead", map[Kind]string{LinkedIn: "https://www.linkedin.com/in/Mixed"})
	if ledger.Held(LinkedIn, "https://www.linkedin.com/in/mixed", "lead") {
		t.Fatal("two LinkedIn URLs that differ in case read as one key")
	}
}

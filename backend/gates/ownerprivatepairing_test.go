// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A table whose visibility admits 'owner' names the owner, or the record it
// marks most-private is the one nobody can read.
//
// The row-scope arm reads the pair as `visibility <> 'owner' OR owner_id = me`.
// A row that says 'owner' and names nobody satisfies neither side, so it is
// readable by no seat at all — its author, an admin, and an unbounded
// row_scope=all principal alike. It is a record destroyed rather than a record
// made private.
//
// The application cannot hold this on its own. contacts/contactvisibility.go
// refuses a PATCH that sets 'owner' while clearing or omitting the owner, and
// that is the right check to have; it cannot refuse the two-step route, because
// it is a rule about a request and this is a property of a record. Narrow with
// an owner named, clear the owner in a second patch, and each write is
// individually admissible while the pair they leave is not (#2137).
//
// So this asks the ESTATE, and asks it of every table rather than of the three
// that have the columns today. `project` currently admits only 'workspace',
// which is strictly stronger and needs no pairing rule — and the day that
// widens, this gate is what says the pairing has to come with it, rather than a
// comment in a migration nobody is reading at the time.

import (
	"bufio"
	"os"
	"regexp"
	"testing"
)

const ownerPairingCatalog = "migrations/testdata/head_catalog.txt"

// visibilityAdmitsOwner matches the enum CHECK a visibility column carries, and
// only when 'owner' is one of the values it admits.
var visibilityAdmitsOwner = regexp.MustCompile(
	`^public\.(\w+)\.\w+ CHECK \(\(visibility = ANY .*'owner'::text`)

// namesItsOwner matches the pairing rule in the shape Postgres prints it back,
// which is not the shape it was written in: `visibility <> 'owner' OR owner_id
// IS NOT NULL` comes back with its own parenthesisation, so this reads the
// parts rather than the spelling.
var namesItsOwner = regexp.MustCompile(
	`^public\.(\w+)\.\w+ CHECK \(\(\(visibility <> 'owner'::text\) OR \(owner_id IS NOT NULL\)\)\)`)

// hasOwnerColumn is the third fact: a table can only be asked to name an owner
// if it has the column to name one in.
var hasOwnerColumn = regexp.MustCompile(`^public\.(\w+)\.owner_id `)

func TestEveryOwnerPrivateTableNamesItsOwner(t *testing.T) {
	t.Parallel()
	admitsOwner, pairs, owned := map[string]bool{}, map[string]bool{}, map[string]bool{}

	file, err := os.Open(ownerPairingCatalog)
	if err != nil {
		t.Fatalf("opening %s: %v", ownerPairingCatalog, err)
	}
	//craft:ignore swallowed-errors a read-only close on a file this test is finished with; the read itself is asserted above
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := visibilityAdmitsOwner.FindStringSubmatch(line); m != nil {
			admitsOwner[m[1]] = true
		}
		if m := namesItsOwner.FindStringSubmatch(line); m != nil {
			pairs[m[1]] = true
		}
		if m := hasOwnerColumn.FindStringSubmatch(line); m != nil {
			owned[m[1]] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", ownerPairingCatalog, err)
	}

	// A census that recognised no owner-private table would report the tree
	// clean in the same words as a tree with nothing to fix, and this one reads
	// a generated file whose format is not this gate's to control.
	if len(admitsOwner) < 3 {
		t.Fatalf("found %d table(s) admitting visibility='owner' and expected at least 3 — this "+
			"census has stopped recognising the enum CHECK rather than the tree having lost them: %v",
			len(admitsOwner), sorted(admitsOwner))
	}

	for _, table := range sorted(admitsOwner) {
		if !owned[table] {
			// Admitting 'owner' with no owner_id at all is a different and
			// larger defect, and reporting it as a missing CHECK would send
			// the reader to add one that cannot be written.
			t.Errorf("%s admits visibility='owner' and has no owner_id column, so the row-scope arm "+
				"can never match a caller: every row it marks private is readable by nobody", table)
			continue
		}
		if pairs[table] {
			continue
		}
		t.Errorf("%s admits visibility='owner' and does not require an owner with it. A row that says "+
			"'owner' and names nobody satisfies neither side of the row-scope arm, so it is readable "+
			"by NO seat — its author, an admin, and an unbounded principal alike — and no patch "+
			"through the endpoint that produced it can reach it to repair it.\n"+
			"  Add `CHECK (visibility <> 'owner' OR owner_id IS NOT NULL)` in a migration, with a "+
			"backfill for any row already in that state.", table)
	}
}

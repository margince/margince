// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A forecast finding names the deal it is about, so a manager reading the
// review can open it. The row carried a subject_kind and a uuid, which named
// the record to the database and to nobody else.

func exception(kind, label string) Exception {
	return Exception{
		ID: ids.NewV7(), Type: "amount_disagrees", SubjectKind: kind,
		SubjectID: ids.NewV7(), Severity: "high", Status: "open",
		SubjectLabel: label,
	}
}

func TestAFindingNamesTheDealItIsAbout(t *testing.T) {
	t.Parallel()
	in := exception("deal", "Fleet retrofit")

	wire := exceptionsToWire([]Exception{in})

	if len(wire) != 1 {
		t.Fatalf("got %d rows, want one", len(wire))
	}
	subject := wire[0].Subject
	if subject == nil {
		t.Fatal("the finding names no record, so the review row opens nothing")
	}
	if subject.Type != crmcontracts.AttentionSubjectTypeDeal {
		t.Errorf("subject type = %q, want deal", subject.Type)
	}
	if ids.UUID(subject.Id) != in.SubjectID {
		t.Errorf("subject names %s, want the finding's own subject %s", subject.Id, in.SubjectID)
	}
	if subject.Label == nil || *subject.Label != "Fleet retrofit" {
		t.Errorf("subject label = %v, want the deal's own name", subject.Label)
	}
	// The durable fields stay, so a client that has them and not the subject
	// still draws today's row.
	if wire[0].SubjectId != subject.Id {
		t.Errorf("subject_id = %s, want the same record the subject names", wire[0].SubjectId)
	}
}

// A finding whose name this reader never saw carries the id alone. The label
// comes from the surface read's own join under the caller's deal scope, so an
// empty one is a record they were not shown rather than a record with no name.
func TestAFindingWithNoNameCarriesNoSubject(t *testing.T) {
	t.Parallel()
	wire := exceptionsToWire([]Exception{exception("deal", "")})

	if wire[0].Subject != nil {
		t.Errorf("an unnamed finding produced a subject: %+v", *wire[0].Subject)
	}
	if wire[0].SubjectId == (crmcontracts.InputCheck{}).SubjectId {
		t.Error("the durable subject_id went missing too; the row can still say WHICH id")
	}
}

// A kind the subject vocabulary has no screen for yields no subject. An offer,
// a contract and a signal are not records a client routes to, and a link that
// opens nothing teaches a reader that the row's links do not work.
func TestAFindingOnAKindWithNoScreenNamesNoSubject(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"offer", "contract", "signal"} {
		wire := exceptionsToWire([]Exception{exception(kind, "Named anyway")})
		if wire[0].Subject != nil {
			t.Errorf("a %s finding produced a subject: %+v", kind, *wire[0].Subject)
		}
	}
}

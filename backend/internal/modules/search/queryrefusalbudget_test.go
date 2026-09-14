// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// A refusal that names a closed set must be able to DELIVER that set, and a
// caller must not be able to push it out.
//
// Every refusal here puts the caller's own token first and the remedy after it,
// and httperr cuts the whole message at MaxFaultText before any surface sees
// it. So an invented field name long enough ate the sentence saying what to do
// about it, and the operator refusal — which names a field's whole operator set
// — lost the set. A truncated set is worse than an absent one: it reads as
// complete, so a caller stops looking for the entry that was removed.
//
// compose has this census for its own vocabularies and says in its own header
// that it cannot reach this package's. This is that obligation, held here.
//
// TWO CLAIMS, and the second is the premise of the first:
//
//   - a flooded caller token leaves the remedy and the set intact;
//   - every member of this package's own vocabularies renders WITHIN the caller
//     bound, so quoting one back is lossless. Without it the first claim would
//     be bought by truncating the answer instead of the question.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// floodedToken is what a caller sends when the refusal's budget is the thing
// under attack: far past any bound, so a producer that does not bound its echo
// leaves nothing for the remedy.
const floodedToken = 4000

// TestTheRemedySurvivesAFloodedCallerToken drives the real producers and reads
// the CLASSIFIED fault rather than the raw message.
//
// The renderer is not the only ceiling: httperr bounds a module-declared fault
// at MaxFaultText before any surface is reached, so a test measuring the raw
// string would see a remedy the caller never gets. Asking httperr.Classify is
// asking the surface rather than a model of it.
func TestTheRemedySurvivesAFloodedCallerToken(t *testing.T) {
	t.Parallel()
	vocab := TargetVocabulary{
		Target: "contact",
		Fields: []Field{newField("full_name", KindText), newField("created_at", KindTimestamp)},
	}
	flood := strings.Repeat("k", floodedToken)

	for _, tc := range []struct {
		name   string
		clause Predicate
		remedy string
		// set is what the refusal must still be able to say, beyond the remedy:
		// the closed vocabulary a caller acts on. Empty where the refusal names
		// no set and the remedy IS the whole answer.
		set []string
	}{
		{
			name:   "an invented field name",
			clause: Predicate{Field: flood, Op: OpEq, Value: json.RawMessage(`"x"`)},
			remedy: "read margince://schema/query for the fields available to you",
		},
		{
			name:   "an invented operator on a real field",
			clause: Predicate{Field: "full_name", Op: flood, Value: json.RawMessage(`"x"`)},
			remedy: "is not one of them",
			set:    operatorsByKind[KindText],
		},
		{
			name:   "both halves flooded at once",
			clause: Predicate{Field: flood, Op: flood, Value: json.RawMessage(`"x"`)},
			remedy: "read margince://schema/query for the fields available to you",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var plan ValidatedPlan
			refusals := checkPredicates(vocab, "where", []Predicate{tc.clause}, &plan)
			if len(refusals) != 1 {
				t.Fatalf("the flood produced %d refusal(s), want exactly 1: %+v", len(refusals), refusals)
			}
			fault, classified := httperr.Classify(&PlanRefusal{Refusals: refusals})
			if !classified {
				t.Fatalf("the refusal is outside the taxonomy, so a caller receives an opaque 500: %+v", refusals)
			}
			if len(fault.Fields) != 1 {
				t.Fatalf("the fault carries %d field entries, want 1: %+v", len(fault.Fields), fault.Fields)
			}
			message := fault.Fields[0].Message
			if !strings.Contains(message, tc.remedy) {
				t.Errorf("the remedy is gone from the refusal — the caller keeps their own mistake and "+
					"loses the answer.\n  want to contain: %s\n  got (%d bytes): %s",
					tc.remedy, len(message), message)
			}
			for _, member := range tc.set {
				if !strings.Contains(message, httperr.QuoteCaller(member)) {
					t.Errorf("%q is missing from the operator set the refusal names — a truncated set "+
						"reads as complete, so a caller stops looking for what was removed.\n  got "+
						"(%d bytes): %s", member, len(message), message)
				}
			}
			// The bound is what makes the above possible, so it is asserted
			// rather than inferred from the two passing: a producer that stopped
			// bounding would pass the remedy check on a message httperr then
			// cuts, and the failure would appear one layer away.
			if len(message) > httperr.MaxFaultText {
				t.Errorf("the delivered refusal is %d bytes, past httperr's own %d — which means the "+
					"bound moved rather than that this passed", len(message), httperr.MaxFaultText)
			}
			if strings.HasSuffix(message, "…") {
				t.Errorf("the delivered refusal ends in an ellipsis, so httperr cut it: the caller "+
					"keeps their own token and loses whatever came after it.\n  got (%d bytes): %s",
					len(message), message)
			}
		})
	}
}

// TestEveryVocabularyMemberFitsTheCallerBound is the premise the test above
// rests on. Quoting is lossless only for a name inside the bound; a member
// longer than it would be quoted back TRUNCATED and match nothing in the set
// printed beside it, which is the same defect wearing the server's name instead
// of the caller's.
//
// The operators are derived from the map that defines them rather than listed,
// so a kind that gains one is covered the day it does.
func TestEveryVocabularyMemberFitsTheCallerBound(t *testing.T) {
	t.Parallel()
	members := []string{PlanVersion}
	for _, ops := range operatorsByKind {
		members = append(members, ops...)
	}
	for kind := range operatorsByKind {
		members = append(members, string(kind))
	}
	if len(members) < len(operatorsByKind) {
		t.Fatalf("the census gathered %d member(s) from %d kind(s) — it has stopped reaching this "+
			"package's vocabularies and would pass over nothing",
			len(members), len(operatorsByKind))
	}
	for _, member := range members {
		if quoted := httperr.QuoteCaller(member); strings.Contains(quoted, "…") {
			t.Errorf("%q renders as %s — a server-side name past the caller bound comes back "+
				"truncated, matching nothing in the set beside it", member, quoted)
		}
	}
}

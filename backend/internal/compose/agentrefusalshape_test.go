// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One mistake, one machine code, whichever credential presented it — and a
// refusal that names an input says so in the structured field list rather than
// only in prose.
//
// The governed-call seam moved a set of argument guards onto the REST agent
// door that had run only on the tool door. They refuse the right calls; what
// they ANSWERED with had drifted from what the contract and the session door
// answer for the same mistake, which is the divergence that whole change exists
// to remove.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fieldsNamed reads the per-field entries a refusal carries on the wire.
func fieldsNamed(t *testing.T, err error) []struct{ Field, Code string } {
	t.Helper()
	rec := httptest.NewRecorder()
	httperr.Write(rec, httptest.NewRequest(http.MethodPost, "/v1/probe", http.NoBody), err)
	var problem struct {
		Status  int `json:"status"`
		Details struct {
			Errors []struct{ Field, Code string } `json:"errors"`
		} `json:"details"`
	}
	if uErr := json.Unmarshal(rec.Body.Bytes(), &problem); uErr != nil {
		t.Fatalf("the refusal is not a problem document: %v\n%s", uErr, rec.Body.String())
	}
	if problem.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", problem.Status, rec.Body.String())
	}
	return problem.Details.Errors
}

// A BadArgsError that names an input reaches the caller as a per-field entry.
//
// It declared MessageFault, which apperrors documents as "a refusal that names
// NO input the caller can change" — so a refusal like `to_phase "vibing" is not
// a project phase` answered a 422 whose `details.errors` was empty and whose
// only actionable content was prose. A client that branches on the structured
// list had nothing to branch on, and the coverage gate had to match a substring
// instead of a field code.
func TestABadArgumentNamingAnInputAnswersAPerFieldEntry(t *testing.T) {
	entries := fieldsNamed(t, &agents.BadArgsError{
		Field: "to_phase",
		Cause: errProbeBadPhase,
	})
	if len(entries) != 1 || entries[0].Field != "to_phase" || entries[0].Code != "validation_error" {
		t.Fatalf("the refusal names %+v, want one entry for to_phase/validation_error", entries)
	}
}

// And one that names none still carries no entry. Inventing one would point a
// caller at an argument that is not theirs to change, which is the reason the
// message form exists at all — so the field is optional and its absence is an
// answer rather than an omission.
func TestABadArgumentNamingNoInputCarriesNoEntry(t *testing.T) {
	if entries := fieldsNamed(t, &agents.BadArgsError{Cause: errProbeBadPhase}); len(entries) != 0 {
		t.Fatalf("the refusal names %+v, want none", entries)
	}
}

// A merge with no `target_id` is the caller's own input, and answers as one.
//
// The zero id used to travel on to the resolver, which read it and returned
// not-found — so an agent got a 404 where a session on the same route gets the
// handler's 422. Existence-hiding wins where a ROUTED id names a record, and a
// body field the caller left out is not one.
func TestAMergeWithNoTargetNamesTheFieldRatherThanHidingARecord(t *testing.T) {
	// A ROUTED request, so the source id resolves and the only thing missing is
	// the one this case is about.
	_, err := mergeCommand(
		agentPolicy{Op: "mergeCompany", RecordType: recordTypeCompany},
		restCommandDeps{records: seamRecord{}},
		patchRequest("/v1/companies", ids.NewV7(), []byte(`{}`)),
		[]byte(`{}`),
	)
	if err == nil {
		t.Fatal("a merge naming no survivor was accepted")
	}
	entries := fieldsNamed(t, err)
	if len(entries) != 1 || entries[0].Field != "target_id" {
		t.Fatalf("the refusal names %+v, want one entry for target_id — a 404 here tells an agent a "+
			"record is invisible when what it did was omit an argument", entries)
	}
}

// A replacement rune is attributed to the member that carries it.
//
// json.Marshal coerces the path, the body and the header values into one
// canonical object, so a scan of the finished object reported every one of them
// as `body` — and a caller told to fix a body that is already clean has been
// handed a task it cannot perform.
func TestAReplacementRuneNamesTheMemberItCameFrom(t *testing.T) {
	// An escaped unpaired surrogate: valid UTF-8 on the wire, and U+FFFD once
	// encoded — which is why the raw-byte check upstream cannot see it.
	const surrogate = `"\udcff"`

	for _, tc := range []struct {
		name  string
		path  string
		body  []byte
		field string
	}{
		{"in the body", "/v1/people/x", []byte(`{"note":` + surrogate + `}`), "body"},
		{"in the path", "/v1/people/�", []byte(`{}`), "path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := canonicalRESTCall("updatePerson", tc.path, http.Header{}, tc.body, keySettledByThisCall)
			if err == nil {
				t.Fatal("a call carrying the replacement character was accepted — two different calls hash alike")
			}
			entries := fieldsNamed(t, err)
			if len(entries) != 1 || entries[0].Field != tc.field {
				t.Fatalf("the refusal names %+v, want one entry for %q", entries, tc.field)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.field) {
				t.Errorf("the sentence reads %q and does not name %q either", err.Error(), tc.field)
			}
		})
	}
}

// errProbeBadPhase is a refusal of the shape this file is about: it names an
// input in its prose, which is exactly the case the field must now carry too.
var errProbeBadPhase = errors.New(`to_phase "vibing" is not a project phase`)

// The detail a field-fault refusal carries is bounded like every other value
// on that path.
//
// It was the one that was not, and it is the value most likely to be long: a
// BadArgsError's Guidance is deliberately unbounded — bounding it with the echo
// truncated the accepted-field list mid-word, deleting the actionable half of a
// message whose reader had just proved they did not know the vocabulary — and
// Error() concatenates the two.
func TestAFieldFaultsDetailIsBounded(t *testing.T) {
	rec := httptest.NewRecorder()
	httperr.Write(rec, httptest.NewRequest(http.MethodPost, "/v1/probe", http.NoBody),
		&agents.BadArgsError{
			Field:    "fields",
			Cause:    errProbeBadPhase,
			Guidance: "accepts " + strings.Repeat("a_long_field_name, ", 80),
		})
	var problem struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the refusal is not a problem document: %v", err)
	}
	// The bound plus the ellipsis boundFaultText appends, which is what every
	// other value on this path measures to as well.
	const ellipsis = len("…")
	if len(problem.Detail) > httperr.MaxFaultText+ellipsis {
		t.Errorf("detail is %d bytes, above the %d-byte bound every other value on this path takes",
			len(problem.Detail), httperr.MaxFaultText)
	}
	if !strings.HasSuffix(problem.Detail, "…") {
		t.Error("the detail was not bounded at all — this case passes on a short message whatever the code does")
	}
}

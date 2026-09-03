// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeReportVocabulary struct {
	doc  string
	err  error
	runs int
}

func (f *fakeReportVocabulary) ReportVocabularyDocument(context.Context) (json.RawMessage, error) {
	f.runs++
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(f.doc), nil
}

// The vocabulary comes back VERBATIM. Anything this tool did to the document
// beyond passing it through would be a second rendering of what the catalog
// derives — the copy the whole move removed.
func TestTheReportVocabularyIsPassedThroughUnchanged(t *testing.T) {
	const doc = `{"version":"1","reports":[{"report":"deals-by-stage","filters":["owner_id"]}]}`
	read := &fakeReportVocabulary{doc: doc}

	out, err := describeReportVocabulary{read: read}.Handle(context.Background(), nil)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var result DescribeReportVocabularyResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	if got := string(result.Vocabulary); got != doc {
		t.Errorf("vocabulary = %s, want it byte-for-byte as composed:\n%s", got, doc)
	}
	if read.runs != 1 {
		t.Errorf("the reader ran %d times, want exactly 1", read.runs)
	}
}

// A reader that refuses says so, rather than answering an empty vocabulary.
//
// This is the overlay path in production: a workspace whose system of record is
// an incumbent refuses run_report, and the guard refuses this with it. An empty
// document would read as "this installation has no reports", which is a false
// statement about the product rather than a refusal.
func TestARefusedReadIsNotAnEmptyReportVocabulary(t *testing.T) {
	boom := errors.New("this workspace's system of record serves no reports")
	_, err := describeReportVocabulary{read: &fakeReportVocabulary{err: boom}}.
		Handle(context.Background(), nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the reader's own refusal", err)
	}
}

// The tool takes no arguments, and says so in its schema. A `report` filter
// would only let a caller narrow what it already receives, at the cost of a
// name it could spell wrong — the failure this tool exists to end.
func TestTheReportVocabularyToolAsksForNothing(t *testing.T) {
	spec := describeReportVocabulary{read: &fakeReportVocabulary{doc: `{}`}}.Spec()
	// A map rather than a struct: the JSON Schema keyword is
	// `additionalProperties`, which no snake_case tag can spell.
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatalf("decoding the input schema: %v", err)
	}
	var properties map[string]any
	if raw, ok := schema["properties"]; ok {
		if err := json.Unmarshal(raw, &properties); err != nil {
			t.Fatalf("decoding the schema's properties: %v", err)
		}
	}
	var additional *bool
	if raw, ok := schema["additionalProperties"]; ok {
		if err := json.Unmarshal(raw, &additional); err != nil {
			t.Fatalf("decoding additionalProperties: %v", err)
		}
	}
	if len(properties) != 0 {
		t.Errorf("the tool advertises %d argument(s); it takes none", len(properties))
	}
	if required, ok := schema["required"]; ok && len(required) != 0 {
		t.Errorf("the tool requires %s; it takes no arguments", required)
	}
	if additional == nil || *additional {
		t.Error("the schema admits extra properties, so a misspelled argument would pass silently")
	}
}

// An argument the schema forbids is REFUSED, not ignored.
//
// The handler reads no arguments, which makes it tempting to skip decoding them
// — and skipping it makes `additionalProperties: false` a promise nothing
// keeps. A call carrying {"report":"deals-by-stage"} would be accepted silently
// and the caller would then read the whole catalog believing it asked about one
// report.
func TestAnArgumentTheReportVocabularySchemaForbidsIsRefused(t *testing.T) {
	read := &fakeReportVocabulary{doc: `{}`}
	_, err := describeReportVocabulary{read: read}.
		Handle(context.Background(), json.RawMessage(`{"report":"deals-by-stage"}`))
	if err == nil {
		t.Fatal("a forbidden argument was accepted, so the caller was answered a question it did not ask")
	}
	if !strings.Contains(err.Error(), "report") {
		t.Errorf("the refusal does not name the key it refused: %v", err)
	}
	if read.runs != 0 {
		t.Error("the vocabulary was composed for a call that should never have reached it")
	}
	// The call the tool IS for — no payload at all — still works. decodeArgs
	// would answer "the payload is empty; send a JSON object carrying this
	// operation's fields", which is advice for a tool that has fields.
	if _, err := (describeReportVocabulary{read: read}).Handle(context.Background(), nil); err != nil {
		t.Fatalf("the argument-free call this tool exists for was refused: %v", err)
	}
}

// The tool names the resource it stands in for, and the tool that consumes what
// it describes, so a reader of either surface knows they are one document.
func TestTheReportVocabularyToolNamesTheResourceItMirrors(t *testing.T) {
	spec := describeReportVocabulary{read: &fakeReportVocabulary{doc: `{}`}}.Spec()
	if !strings.Contains(spec.Description, ReportVocabularyURI) {
		t.Errorf("the description does not name %s, so a client reading both surfaces cannot "+
			"tell they carry the same document", ReportVocabularyURI)
	}
	if !strings.Contains(spec.Description, "run_report") {
		t.Error("the description does not name run_report, so a caller learns the vocabulary " +
			"without learning what to do with it")
	}
}

// run_report's own description NAMES the document and does not order a read.
//
// Both halves matter and they pull against each other. Without the name, the
// recital's removal leaves a caller with refusals and no route; with an
// imperative, a binding reads "read this first" on a goal that needs no report
// and spends a step on it — measured, and the reason
// TestNoToolOrdersTheModelToReadADocument exists.
func TestRunReportNamesTheDocumentWithoutOrderingARead(t *testing.T) {
	described := describeReportCatalog(twoReportCatalog())
	if !strings.Contains(described, ReportVocabularyURI) {
		t.Errorf("run_report's `report` description does not name %s, so a caller whose plan is "+
			"refused has no route to the names: %s", ReportVocabularyURI, described)
	}
	if !strings.Contains(described, "describe_report_vocabulary") {
		t.Errorf("it does not name the door either, and the Surface-B runner can reach nothing "+
			"else: %s", described)
	}
	// The refusal fact is what makes deferring the recital safe, so it is
	// stated rather than assumed.
	if !strings.Contains(described, "refused") {
		t.Errorf("it does not say a wrong name is refused, which is the fact that makes reading "+
			"the document optional rather than mandatory: %s", described)
	}
	for _, imperative := range []string{"read this", "read it", "you must read", "before your first"} {
		if strings.Contains(strings.ToLower(described), imperative) {
			t.Errorf("it orders a read (%q): %s", imperative, described)
		}
	}
	// And the recital itself is GONE. This is the saving: a description that
	// still listed one report's names would be the same second copy in a
	// shorter font.
	for _, recited := range []string{"stage_id", "owner_id", "amount_minor"} {
		if strings.Contains(described, recited) {
			t.Errorf("the description still recites the vocabulary name %q, so the tokens the "+
				"move bought are still being spent: %s", recited, described)
		}
	}
}

// An installation with no prebuilt reports says so, and names no document.
// Pointing at a vocabulary of nothing is worse than silence: the caller spends
// a call to learn the tool cannot be used.
func TestAnEmptyCatalogDescribesNoDocument(t *testing.T) {
	described := describeReportCatalog(nil)
	if strings.Contains(described, ReportVocabularyURI) {
		t.Errorf("an installation with no reports still points at a vocabulary: %s", described)
	}
	if !strings.Contains(described, "No prebuilt report") {
		t.Errorf("it does not say the catalog is empty: %s", described)
	}
}

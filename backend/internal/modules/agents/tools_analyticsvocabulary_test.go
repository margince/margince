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

type fakeAnalyticsVocabulary struct {
	doc  string
	err  error
	runs int
}

func (f *fakeAnalyticsVocabulary) AnalyticsVocabularyDocument(context.Context) (string, error) {
	f.runs++
	if f.err != nil {
		return "", f.err
	}
	return f.doc, nil
}

// The vocabulary comes back VERBATIM. Anything this tool did to the document
// beyond passing it through would be a second rendering of what the engine
// derives per caller — the copy the whole move removed.
func TestTheAnalyticsVocabularyIsPassedThroughUnchanged(t *testing.T) {
	const doc = "schema version abc123\npipeline-current\n  group by: stage\n  measure:  value\n"
	read := &fakeAnalyticsVocabulary{doc: doc}

	out, err := describeAnalyticsVocabulary{read: read}.Handle(context.Background(), nil)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var result DescribeAnalyticsVocabularyResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	if result.Vocabulary != doc {
		t.Errorf("vocabulary = %q, want it byte-for-byte as composed: %q", result.Vocabulary, doc)
	}
	if read.runs != 1 {
		t.Errorf("the reader ran %d times, want exactly 1", read.runs)
	}
}

// A reader that refuses says so, rather than answering an empty vocabulary.
//
// This is the overlay path in production: a workspace whose system of record
// is an incumbent refuses the analytics verb, and the guard refuses this with
// it. An empty document would read as "this seat may measure nothing", which
// is a false statement about the product rather than a refusal.
func TestARefusedReadIsNotAnEmptyAnalyticsVocabulary(t *testing.T) {
	boom := errors.New("this workspace's system of record serves no analytics")
	_, err := describeAnalyticsVocabulary{read: &fakeAnalyticsVocabulary{err: boom}}.
		Handle(context.Background(), nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the reader's own refusal", err)
	}
}

// The tool takes no arguments, and says so in its schema. A population filter
// would only let a caller narrow what it already receives, at the cost of a
// name it could spell wrong — the failure this tool exists to end.
func TestTheAnalyticsVocabularyToolAsksForNothing(t *testing.T) {
	spec := describeAnalyticsVocabulary{read: &fakeAnalyticsVocabulary{doc: "x"}}.Spec()
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

// An argument the schema forbids is REFUSED, not ignored — and the call the
// tool IS for, no payload at all, still works.
func TestAnArgumentTheAnalyticsVocabularySchemaForbidsIsRefused(t *testing.T) {
	read := &fakeAnalyticsVocabulary{doc: "x"}
	_, err := describeAnalyticsVocabulary{read: read}.
		Handle(context.Background(), json.RawMessage(`{"entity":"pipeline-current"}`))
	if err == nil {
		t.Fatal("a forbidden argument was accepted, so the caller was answered a question it did not ask")
	}
	if !strings.Contains(err.Error(), "entity") {
		t.Errorf("the refusal does not name the key it refused: %v", err)
	}
	if read.runs != 0 {
		t.Error("the vocabulary was composed for a call that should never have reached it")
	}
	if _, err := (describeAnalyticsVocabulary{read: read}).Handle(context.Background(), nil); err != nil {
		t.Fatalf("the argument-free call this tool exists for was refused: %v", err)
	}
}

// The tool names the resource it stands in for, and the tool that consumes
// what it describes, so a reader of either surface knows they are one
// document.
func TestTheAnalyticsVocabularyToolNamesTheResourceItMirrors(t *testing.T) {
	spec := describeAnalyticsVocabulary{read: &fakeAnalyticsVocabulary{doc: "x"}}.Spec()
	if !strings.Contains(spec.Description, "margince://schema/analytics") {
		t.Error("the description does not name margince://schema/analytics, so a client reading " +
			"both surfaces cannot tell they carry the same document")
	}
	if !strings.Contains(spec.Description, "run_analytics_query") {
		t.Error("the description does not name run_analytics_query, so a caller learns the " +
			"vocabulary without learning what to do with it")
	}
}

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

type fakeVocabulary struct {
	doc  string
	err  error
	runs int
}

func (f *fakeVocabulary) VocabularyDocument(context.Context) (json.RawMessage, error) {
	f.runs++
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(f.doc), nil
}

// The vocabulary comes back VERBATIM, not restated.
//
// The document is composed per caller from the field catalog and the live
// column catalog. Anything this tool did to it beyond passing it through would
// be a second rendering of exactly what is derived — the hand-maintained copy
// this surface refuses to keep, and the reason the grammar was never inlined
// into query_workspace's input schema either.
func TestTheVocabularyIsPassedThroughUnchanged(t *testing.T) {
	const doc = `{"version":"v1","targets":[{"target":"organization",` +
		`"fields":[{"name":"address","kind":"geo","ops":["within_radius"]}]}]}`
	read := &fakeVocabulary{doc: doc}

	out, err := describeQueryVocabulary{read: read}.Handle(context.Background(), nil)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var result DescribeQueryVocabularyResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	if got := string(result.Vocabulary); got != doc {
		t.Errorf("vocabulary = %s, want it byte-for-byte as composed:\n%s", got, doc)
	}
	if read.runs != 1 {
		t.Errorf("the resolver ran %d times, want exactly 1", read.runs)
	}
}

// A resolver that fails says so, rather than answering an empty vocabulary.
//
// An empty document reads as "this workspace admits nothing", which would send
// a caller to give up on a question it could have asked.
func TestAFailedResolveIsNotAnEmptyVocabulary(t *testing.T) {
	boom := errors.New("the column catalog is unreachable")
	_, err := describeQueryVocabulary{read: &fakeVocabulary{err: boom}}.
		Handle(context.Background(), nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the resolver's own failure", err)
	}
}

// The tool takes no arguments, and says so in its schema.
//
// A target filter would only let a caller narrow what they already receive, at
// the cost of a name they could spell wrong — which is the exact failure this
// tool exists to end.
func TestTheVocabularyToolAsksForNothing(t *testing.T) {
	spec := describeQueryVocabulary{read: &fakeVocabulary{doc: `{}`}}.Spec()
	// A map rather than a struct: the JSON Schema keyword is
	// `additionalProperties`, which no snake_case tag can spell, and the
	// repo's tag linter is right to insist on snake_case for wire types.
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

// The tool names the resource it stands in for, so a reader of either knows
// they are one document.
func TestTheVocabularyToolNamesTheResourceItMirrors(t *testing.T) {
	spec := describeQueryVocabulary{read: &fakeVocabulary{doc: `{}`}}.Spec()
	if !strings.Contains(spec.Description, "margince://schema/query") {
		t.Error("the description does not name margince://schema/query, so a client reading " +
			"both surfaces cannot tell they carry the same document")
	}
	// And it points at the tool that consumes what it describes. A vocabulary
	// with no named consumer is a document a caller has no reason to fetch.
	if !strings.Contains(spec.Description, "query_workspace") {
		t.Error("the description does not name query_workspace, so a caller learns the " +
			"vocabulary without learning what to do with it")
	}
}

// A composition wiring no reader registers no tool.
//
// A surface advertising a door to nothing is worse than one door fewer: the
// caller spends a round trip to be told the thing does not work here.
func TestNoReaderRegistersNoTool(t *testing.T) {
	r := NewRegistry(nil, nil)
	RegisterVocabularyTool(r, nil)
	for _, spec := range r.Specs() {
		if spec.Name == "describe_query_vocabulary" {
			t.Fatal("the tool registered with no vocabulary behind it")
		}
	}
}

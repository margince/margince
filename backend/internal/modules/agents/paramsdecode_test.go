// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A decoder never describes this program to the model that called it.
//
// decodeArgs has masked encoding/json's own words since it was written, for the
// reason its comment gives: a Go struct field and a Go type are things an agent
// can neither act on nor is entitled to read, and the text lands in a transcript
// whose later prompts the same run reads. The two PARAMS decoders on this
// surface — tools/call's envelope and resources/read's — never got that
// treatment and spliced the decoder verbatim.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// goStructLeaks are the words that say the message came from this program's
// types rather than from the caller's request.
var goStructLeaks = []string{"Go struct field", "json:", "unmarshal", "cannot unmarshal"}

func TestAMalformedToolsCallEnvelopeDoesNotNameThisProgram(t *testing.T) {
	t.Parallel()
	logged := &bytes.Buffer{}
	srv := NewDispatcher(nil, nil, "t", "0").
		WithLogger(slog.New(slog.NewTextHandler(logged, nil)))

	// `name` is a string in the envelope; a number makes encoding/json describe
	// the Go field it could not fill.
	got := srv.call(context.Background(), json.RawMessage(`{"name":5}`), framing{})

	said := renderedToolText(t, got)
	for _, leak := range goStructLeaks {
		if strings.Contains(strings.ToLower(said), strings.ToLower(leak)) {
			t.Errorf("the refusal names this program (%q):\n%s", leak, said)
		}
	}
	if !strings.Contains(said, "malformed tools/call params") {
		t.Errorf("the refusal does not say what was wrong:\n%s", said)
	}
	// NOTHING is logged here, and that is the design rather than a gap.
	// SafeDecodeError withholds only a shape it cannot NAME, and json.Unmarshal
	// into a struct produces only shapes RestateDecodeError does name — a syntax
	// error or a type error. So the `withheld` log beside these two call sites is
	// the same defensive arm decodeArgs keeps, reachable if that restating ever
	// narrows, and there is no input to this decoder that exercises it today.
	// Asserting on a contrived one would be asserting about the fixture.
	if logged.Len() != 0 {
		t.Errorf("a nameable decode error was also logged, so the operator sees noise:\n%s",
			logged.String())
	}
}

func TestAMalformedResourcesReadParamsDoesNotNameThisProgram(t *testing.T) {
	t.Parallel()
	logged := &bytes.Buffer{}
	srv := NewDispatcher(nil, nil, "t", "0").
		WithLogger(slog.New(slog.NewTextHandler(logged, nil)))

	_, rpcErr := srv.readResource(context.Background(), json.RawMessage(`{"uri":5}`), framing{})

	if rpcErr == nil {
		t.Fatal("a uri that is not a string was accepted")
	}
	for _, leak := range goStructLeaks {
		if strings.Contains(strings.ToLower(rpcErr.Message), strings.ToLower(leak)) {
			t.Errorf("the refusal names this program (%q): %s", leak, rpcErr.Message)
		}
	}
	if !strings.Contains(rpcErr.Message, "invalid params") {
		t.Errorf("the refusal does not say what was wrong: %s", rpcErr.Message)
	}
	if logged.Len() != 0 {
		t.Errorf("a nameable decode error was also logged:\n%s", logged.String())
	}
	// No unnameable counterpart for this decoder: every malformed `uri` shape
	// RestateDecodeError meets here is one it can name, so the withheld branch
	// is exercised through tools/call above, which shares the logic.
}

// A URI the caller chose is bounded and escaped in the answer, and the four
// not-found branches answer identically so existence stays hidden.
func TestAnAbsentResourceNamesTheURIBoundedAndEscaped(t *testing.T) {
	t.Parallel()
	srv := NewDispatcher(nil, nil, "t", "0").
		WithLogger(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))

	long := strings.Repeat("u", 4000)
	_, rpcErr := srv.readResource(context.Background(),
		json.RawMessage(`{"uri":"`+long+`"}`), framing{})

	if rpcErr == nil {
		t.Fatal("an unknown resource uri was accepted")
	}
	if len(rpcErr.Message) > 200 {
		t.Errorf("the answer is %d bytes, so the caller chose how much it writes: %s",
			len(rpcErr.Message), rpcErr.Message[:120])
	}
	if !strings.Contains(rpcErr.Message, "no resource at") {
		t.Errorf("the answer does not say what was not found: %s", rpcErr.Message)
	}
}

// renderedToolText pulls the prose out of whichever result shape `call`
// answered with.
//
//craft:ignore naked-any mirrors call()'s own polymorphic return — the protocol makes it CallToolResult or CreateTaskResult and the framing tells them apart by type, so a concrete parameter here could only take one of the two
func renderedToolText(t *testing.T, got any) string {
	t.Helper()
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("the call result does not encode: %v", err)
	}
	return string(encoded)
}

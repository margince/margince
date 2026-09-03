// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The one decode a tool that takes NO arguments performs.
//
// Two tools have this shape — the query vocabulary's door and the report
// vocabulary's — and both declare `{"type":"object","properties":{},
// "additionalProperties":false}`. Spelled once here because the shape has two
// traps and they were both getting re-derived per tool.

import (
	"bytes"
	"encoding/json"
)

// decodeNoArguments enforces the schema of a tool that declares no arguments.
//
// TWO THINGS, and each was a promise the schema made that nothing kept:
//
//   - An ABSENT payload is the normal call. decodeArgs would answer "the payload
//     is empty; send a JSON object carrying this operation's fields", which is
//     advice for a tool that has fields and a refusal of the one call these
//     tools are designed for.
//   - A payload that is not an OBJECT is refused. `null` decodes into
//     `struct{}` without complaint — encoding/json treats it as a no-op — so a
//     caller sending `null` was answered as though it had sent `{}`, against a
//     schema that says the arguments are an object. Anything else non-object
//     (`[]`, `"x"`, `7`) fails the decode already; `null` is the one that slips
//     through, which is why the check is on the shape rather than on the error.
//
// Beyond that it is decodeArgs, so `additionalProperties:false` is enforced the
// way it is everywhere else: a call carrying a member is refused BY NAME rather
// than accepted and ignored.
func decodeNoArguments(in json.RawMessage) error {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] != '{' {
		return &BadArgsError{Cause: errArgumentsNotAnObject}
	}
	var args struct{}
	return decodeArgs(in, &args)
}

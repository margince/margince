// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// PathArguments names the arguments the route carries as `{name}` segments, in
// route order. The served route hands each to the tool as an argument. So an
// agent passes it the same way.
func (v Verb) PathArguments() []string {
	var names []string
	for segment := range strings.SplitSeq(v.Route, "/") {
		if name, ok := strings.CutPrefix(segment, "{"); ok {
			names = append(names, strings.TrimSuffix(name, "}"))
		}
	}
	return names
}

// validatePathArguments holds each path argument to what a path can carry: one
// required primitive the input schema declares. A path segment is never absent,
// so an optional one would publish a call no route can serve.
func (v Verb) validatePathArguments() error {
	names := v.PathArguments()
	if len(names) == 0 {
		return nil
	}
	var doc struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(v.InputSchema, &doc); err != nil {
		return fmt.Errorf("operation %s: InputSchema cannot be read for its path arguments", v.OperationID)
	}
	for i, name := range names {
		if slices.Contains(names[:i], name) {
			return fmt.Errorf("operation %s names the path argument %q twice in %s", v.OperationID, name, v.Route)
		}
		prop, declared := doc.Properties[name]
		if !declared || !slices.Contains(doc.Required, name) {
			return fmt.Errorf("operation %s routes {%s}, which its input schema does not declare as a required argument", v.OperationID, name)
		}
		if !flatArgumentType(prop.Type) {
			return fmt.Errorf("operation %s routes {%s} of type %q — a path segment carries a string, number, integer or boolean", v.OperationID, name, prop.Type)
		}
	}
	return nil
}

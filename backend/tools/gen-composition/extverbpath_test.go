// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

const pathID = "        - name: id\n          in: path\n          required: true\n          schema: {type: string}\n"

// atRecordRoute moves an operation onto /ext/u/quote/{id}.
func atRecordRoute(pathItem string) string {
	return strings.Replace(pathItem, "  /ext/u/quote:", "  /ext/u/quote/{id}:", 1)
}

// A path argument is a property of the tool's input schema. So an agent passes
// the id the way it passes any other argument.
func TestAPathParameterJoinsTheInputSchema(t *testing.T) {
	for name, tc := range map[string]struct{ pathItem, want string }{
		"a GET": {
			pathItem: atRecordRoute(getOperationWith(pathID)),
			want:     `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`,
		},
		"a POST": {
			pathItem: atRecordRoute(strings.Replace(yogiOperation, "      requestBody:", "      parameters:\n"+pathID+"      requestBody:", 1)),
			want:     `{"properties":{"id":{"type":"string"}},"required":["id"],"type":"object"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			verbs, err := verbsInContract("crm.yaml", oneUnit(), contractWith(tc.pathItem))
			if err != nil {
				t.Fatalf("an operation with a path argument must generate: %v", err)
			}
			if got := string(verbs[0].verb.InputSchema); got != tc.want {
				t.Errorf("InputSchema =\n  %s\nwant\n  %s", got, tc.want)
			}
		})
	}
}

func TestAPathParameterTheRouteCannotServeIsRefused(t *testing.T) {
	for name, tc := range map[string]struct{ pathItem, wantErr string }{
		"an optional path parameter": {
			pathItem: atRecordRoute(getOperationWith(strings.Replace(pathID, "required: true", "required: false", 1))),
			wantErr:  "is not required",
		},
		"a path parameter the route does not carry": {
			pathItem: getOperationWith(pathID),
			wantErr:  "is not a {id} segment",
		},
		"a route segment no parameter declares": {
			pathItem: atRecordRoute(getOperationWith("        - name: limit\n          in: query\n          schema: {type: integer}\n")),
			wantErr:  "does not declare as a required argument",
		},
		"a body property that shadows a path parameter": {
			pathItem: atRecordRoute(strings.Replace(
				strings.Replace(yogiOperation, "      requestBody:", "      parameters:\n"+pathID+"      requestBody:", 1),
				"              type: object\n      responses:", "              type: object\n              properties:\n                id: {type: string}\n      responses:", 1)),
			wantErr: "also a path parameter",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := verbsInContract("crm.yaml", oneUnit(), contractWith(tc.pathItem))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestPublishedOperationNoticesGenerateStandardHeaders(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData([]byte(`openapi: 3.1.0
info: {title: Test, version: 1.0.0}
paths:
  /old:
    get:
      operationId: old
      deprecated: true
      x-deprecation:
        release: v1.0.0
        announced_at: '2026-01-31T12:00:00Z'
        sunset_at: '2026-04-30T12:00:00Z'
      responses: {'200': {description: OK}}
`))
	if err != nil {
		t.Fatal(err)
	}
	generated, err := renderDeprecations(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"GET /v1/old"`, `"Deprecation": "@1769860800"`, `"Sunset": "Thu, 30 Apr 2026 12:00:00 GMT"`} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated notice lacks %s: %s", want, generated)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every draft-email route takes the same steering.
//
// The reply route once declared only `intent`, so a re-draft with a changed ask
// could not name the words on screen, and the composer discarded the new draft
// without a word. Four routes each declared their own steering and the one that
// lagged was the one nobody had looked at. The obligation is derived from
// crm.yaml: any operation whose path ends in /draft-email, or whose MCP verb is
// draft_email, builds its request body on DraftSteering, directly or through
// allOf.

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const draftSteeringRef = "#/components/schemas/DraftSteering"

type draftRouteSpec struct {
	Paths map[string]struct {
		Post *draftRouteOp `yaml:"post"`
	} `yaml:"paths"`
}

type draftRouteOp struct {
	OperationID string `yaml:"operationId"`
	MCP         struct {
		Verb string `yaml:"verb"`
	} `yaml:"x-mcp-tool"`
	RequestBody struct {
		Content map[string]struct {
			Schema draftRouteSchema `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"requestBody"`
}

type draftRouteSchema struct {
	Ref   string             `yaml:"$ref"`
	AllOf []draftRouteSchema `yaml:"allOf"`
}

func (s draftRouteSchema) steered() bool {
	if s.Ref == draftSteeringRef {
		return true
	}
	for _, part := range s.AllOf {
		if part.steered() {
			return true
		}
	}
	return false
}

// draftRoutes returns every draft operation as "POST path", and the ones among
// them whose request body is not built on DraftSteering.
func draftRoutes(doc draftRouteSpec) (all, unsteered []string) {
	for path, item := range doc.Paths {
		op := item.Post
		if op == nil || !strings.HasSuffix(path, "/draft-email") && op.MCP.Verb != "draft_email" {
			continue
		}
		id := "POST " + path
		all = append(all, id)
		if !op.RequestBody.Content["application/json"].Schema.steered() {
			unsteered = append(unsteered, id)
		}
	}
	return all, unsteered
}

func readDraftRouteContract(t *testing.T) draftRouteSpec {
	t.Helper()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc draftRouteSpec
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	return doc
}

func TestEveryDraftRouteIsBuiltOnDraftSteering(t *testing.T) {
	t.Parallel()
	all, unsteered := draftRoutes(readDraftRouteContract(t))
	// The contract has four draft routes. Fewer found means the walk stopped
	// reading crm.yaml, which looks the same as every route being fine.
	if len(all) < 4 {
		t.Fatalf("the walk found %d draft routes (%v), and the contract has at least four", len(all), all)
	}
	for _, id := range unsteered {
		t.Errorf("%s does not build its request body on DraftSteering, so it declares its own "+
			"intent and rewrite_of or lacks one. Use allOf: [DraftSteering, {its own fields}]", id)
	}
}

// The gate fails when a route drops the reference, so it is not a pass by
// construction.
func TestADraftRouteWithItsOwnSteeringFailsTheGate(t *testing.T) {
	t.Parallel()
	doc := readDraftRouteContract(t)
	op := doc.Paths["/activities/{id}/draft-email"].Post
	op.RequestBody.Content["application/json"] = struct {
		Schema draftRouteSchema `yaml:"schema"`
	}{}

	_, unsteered := draftRoutes(doc)
	if len(unsteered) != 1 || unsteered[0] != "POST /activities/{id}/draft-email" {
		t.Errorf("unsteered routes = %v, want only the activities route", unsteered)
	}
}

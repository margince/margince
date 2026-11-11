// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// No door records a new standing exception. A rep lets one refused message go
// with the directed send. Rows already on file are only carried by a merge or
// revoked. This fails when a writer comes back. That is a write on the
// exceptions route, an allow action in the agent catalog, or a second insert
// into communication_override.

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	standingExceptionsPath = "/contacts/{id}/consent/allow"
	// standingExceptionCarry is the one statement allowed to write the table:
	// the merge copying live rows onto the surviving record.
	standingExceptionCarry = "internal/modules/consent/overridecarry.go"
)

// overrideInsert matches the statement, whatever its spacing or case.
var overrideInsert = regexp.MustCompile(`(?i)INSERT\s+INTO\s+communication_override\b`)

type standingExceptionOp struct {
	OperationID string         `yaml:"operationId"` //nolint:tagliatelle // OpenAPI's key, not ours to rename
	MCP         map[string]any `yaml:"x-mcp-tool"`  //nolint:tagliatelle // the contract's extension key, not ours to rename
}

// standingExceptionOps reads every operation in the contract, keyed by path and
// then by method. Path-level keys that are not methods decode to empty ops.
func standingExceptionOps(t *testing.T) map[string]map[string]standingExceptionOp {
	t.Helper()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	methods := []string{"get", "put", "post", "patch", "delete"}
	ops := map[string]map[string]standingExceptionOp{}
	for path, item := range doc.Paths {
		for method, node := range item {
			if !slices.Contains(methods, method) {
				continue
			}
			var op standingExceptionOp
			if err := node.Decode(&op); err != nil {
				t.Fatalf("decoding %s %s: %v", method, path, err)
			}
			if ops[path] == nil {
				ops[path] = map[string]standingExceptionOp{}
			}
			ops[path][method] = op
		}
	}
	return ops
}

func TestTheStandingExceptionsRouteOnlyReads(t *testing.T) {
	t.Parallel()
	ops := standingExceptionOps(t)
	route, ok := ops[standingExceptionsPath]
	// The list operation must still be there, or a renamed path would pass here
	// while a writer sat under its new name.
	if !ok || route["get"].OperationID == "" {
		t.Fatalf("%s has no GET in api/crm.yaml: the route this gate guards has moved", standingExceptionsPath)
	}
	for method := range route {
		if method != "get" {
			t.Errorf("%s %s is back: no route records a standing exception; a rep uses directCommunicationSend for one message",
				strings.ToUpper(method), standingExceptionsPath)
		}
	}
	for path, byMethod := range ops {
		for method, op := range byMethod {
			if op.OperationID == "allowContact" || op.MCP["action"] == "allow_contact" {
				t.Errorf("%s %s declares the standing-exception writer (%s)", strings.ToUpper(method), path, op.OperationID)
			}
		}
	}
}

func TestTheAgentCatalogHasNoStandingExceptionWriter(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(publishedCatalog)
	if err != nil {
		t.Fatalf("reading the published catalog: %v", err)
	}
	var catalog struct {
		Tools struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties struct {
						Action struct {
							Enum []string `json:"enum"`
						} `json:"action"`
					} `json:"properties"`
				} `json:"inputSchema"` //nolint:tagliatelle // the MCP catalog's key, not ours to rename
			} `json:"tools"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(source, &catalog); err != nil {
		t.Fatalf("parsing the published catalog: %v", err)
	}
	var actions []string
	for _, tool := range catalog.Tools.Tools {
		if tool.Name == "change_compliance" {
			actions = tool.InputSchema.Properties.Action.Enum
		}
	}
	// revoke_override is the action that stays; without it this read the wrong
	// tool or no tool at all.
	if !slices.Contains(actions, "revoke_override") {
		t.Fatalf("change_compliance in %s lists %v, which lacks revoke_override: the catalog walk read nothing",
			publishedCatalog, actions)
	}
	if slices.Contains(actions, "allow_contact") {
		t.Errorf("change_compliance offers allow_contact again: no action records a standing exception")
	}
}

func TestOnlyTheMergeCarryWritesStandingExceptions(t *testing.T) {
	t.Parallel()
	var writers []string
	for _, root := range []string{"internal", "cmd", "pkg"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			body, err := gatekit.ReadFile(path)
			if err != nil {
				return err
			}
			for range overrideInsert.FindAllIndex(body, -1) {
				writers = append(writers, filepath.ToSlash(path))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if !slices.Contains(writers, standingExceptionCarry) {
		t.Fatalf("no INSERT INTO communication_override found in %s: the census read nothing (found %v)",
			standingExceptionCarry, writers)
	}
	for _, path := range writers {
		if path != standingExceptionCarry {
			t.Errorf("%s inserts into communication_override: only the merge carry may write a standing exception", path)
		}
	}
}

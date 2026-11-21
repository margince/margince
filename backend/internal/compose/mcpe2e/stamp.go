// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Stamp is what one verdict measured. Each part is a digest, or the judge
// model's name, so a verdict whose part no longer matches names the part that
// moved. A verdict also carries the commit and the time it ran, which are
// facts about the run and never compared.
type Stamp struct {
	// Scenario is the scenario file whole: prompt, checks and bar.
	Scenario string `json:"scenario"`
	// Tools is the published entry of every tool the scenario's checks name, a
	// verb by the router that fronts it. It also holds each such router's action
	// catalog: the bodies a model reads before it calls an action.
	Tools string `json:"tools"`
	// Instructions are the server instructions every client's model reads.
	Instructions string `json:"instructions"`
	// Judge is the model that decides the judged criteria, empty when none.
	Judge string `json:"judge"`
	// Harness is the files that build the world and grade the run.
	Harness string `json:"harness"`
}

// Committed files the stamp is read from, relative to the repository root.
// mcp-info.json is the served catalog and instructions, held to the server by
// TestPublishedMCPSurfaceMatchesWhatAClientIsServed.
const (
	mcpInfoFile    = "docs/reference/mcp-info.json"
	routesFile     = "e2e/llm/routes.json"
	candidatesFile = "e2e/llm/candidates.json"
	ScenarioDir    = "e2e/llm/scenarios"
)

// Door is how an assistant reaches Margince, and each door reads its own copy.
type Door string

const (
	// DoorMCP reads the MCP tool entries and the server instructions.
	DoorMCP Door = "mcp"
	// DoorREST reads the REST skill and the shim that offers it as one tool.
	DoorREST Door = "rest"
)

// restTools stand where the MCP tool entries do on the REST door. restSkill
// stands where the server instructions do: the skill a model reads first.
var (
	restTools = []string{"e2e/llm/rest_ops.json", "e2e/llm/restshim.py", "e2e/llm/restmap.py"}
	restSkill = []string{
		"backend/internal/compose/agentbundle/agentcontract_gen.yaml",
		"backend/internal/compose/agentbundle/agentindex_gen.txt",
		"backend/internal/compose/agentbundle/README.md.tmpl",
		"backend/internal/compose/agentbundle/SKILL.md.tmpl",
	}
)

// harnessFiles grade a run or build the world it runs in. A driver is not
// here: how the model was reached is the verdict's folder and `driver`. The
// transcript parser is, since it decides what counts as a call, a refusal and
// the answer; TestTheHarnessHoldsEveryModuleTheGraderImports holds the list.
var harnessFiles = []string{
	"e2e/llm/check.py", "e2e/llm/judge.py", "e2e/llm/endstate.py",
	"e2e/llm/casesetup.py", "e2e/llm/seed-llm-fixtures.sh",
	"e2e/llm/transcript.py", "e2e/llm/mcpclient.py",
}

// judgeTransport are the grader's imports that only reach the judge, whose
// model the stamp names on its own, so a change to them moves no verdict.
var judgeTransport = map[string]string{
	"e2e/llm/candidates.py": "resolves the judge's route; the judge model is its own stamp part",
	"e2e/llm/clean.py":      "starts the judge's CLI with nothing of the operator's",
	"e2e/llm/providers.py":  "the wire to the judge's API",
}

// inputs are the committed files every scenario's stamp is computed from. On
// the REST door restTools is the whole tool layer's digest and tools is empty.
type inputs struct {
	door         Door
	tools        map[string]json.RawMessage
	catalog      map[string]json.RawMessage
	routers      map[string]string
	restTools    string
	instructions string
	judge        string
	harness      string
}

func loadInputs(root string, door Door) (inputs, error) {
	in := inputs{door: door, tools: map[string]json.RawMessage{}, catalog: map[string]json.RawMessage{}, routers: map[string]string{}}
	var err error
	if in.judge, err = canonicalJudge(root); err != nil {
		return inputs{}, err
	}
	if in.harness, err = digestFiles(root, harnessFiles); err != nil {
		return inputs{}, err
	}
	switch door {
	case DoorMCP:
		if err := in.loadMCP(root); err != nil {
			return inputs{}, err
		}
		return in, nil
	case DoorREST:
		if in.restTools, err = digestFiles(root, restTools); err != nil {
			return inputs{}, err
		}
		if in.instructions, err = digestFiles(root, restSkill); err != nil {
			return inputs{}, err
		}
		return in, nil
	default:
		return inputs{}, fmt.Errorf("no door is named %q; it is %s or %s", door, DoorMCP, DoorREST)
	}
}

// loadMCP reads the served catalog, its instructions and the router table.
func (in *inputs) loadMCP(root string) error {
	var info struct {
		Instructions string `json:"instructions"`
		Tools        struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"tools"`
		ActionCatalog map[string]json.RawMessage `json:"action_catalog"`
	}
	if err := readJSON(filepath.Join(root, mcpInfoFile), &info); err != nil {
		return err
	}
	if info.Instructions == "" {
		return fmt.Errorf("%s carries no server instructions to stamp", mcpInfoFile)
	}
	in.instructions = digest([]byte(info.Instructions))
	for _, entry := range info.Tools.Tools {
		// The generator's indentation is not tool copy.
		canonical, err := CanonicalJSON(entry)
		if err != nil {
			return fmt.Errorf("%s: a tool entry: %w", mcpInfoFile, err)
		}
		var named struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(entry, &named); err != nil || named.Name == "" {
			return fmt.Errorf("%s: a tool entry has no name (%v)", mcpInfoFile, err)
		}
		in.tools[named.Name] = canonical
	}
	if len(in.tools) == 0 {
		return fmt.Errorf("%s lists no tools", mcpInfoFile)
	}
	if len(info.ActionCatalog) == 0 {
		return fmt.Errorf("%s publishes no action catalog to stamp", mcpInfoFile)
	}
	for router, answer := range info.ActionCatalog {
		canonical, err := CanonicalJSON(answer)
		if err != nil {
			return fmt.Errorf("%s: the %s action catalog: %w", mcpInfoFile, router, err)
		}
		in.catalog[router] = canonical
	}
	var routes struct {
		Routers map[string]struct {
			Actions map[string]string `json:"actions"`
		} `json:"routers"`
	}
	if err := readJSON(filepath.Join(root, routesFile), &routes); err != nil {
		return err
	}
	for router, table := range routes.Routers {
		for _, verb := range table.Actions {
			in.routers[verb] = router
		}
	}
	return nil
}

// catalogEntry keys a router's action catalog beside its tool entry, under a
// name no tool can take.
const catalogEntry = "actions of "

// CanonicalJSON re-encodes a document in JSON's own key order and layout. A
// digest or a byte comparison then reads the content, not how it was written.
func CanonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	return json.Marshal(parsed)
}

// canonicalJudge is the judge a run uses unless E2E_LLM_JUDGE_MODEL names
// another; the lane files the one that actually judged (e2e/llm/check.py).
func canonicalJudge(root string) (string, error) {
	var table struct {
		Judge struct {
			Routes map[string]struct {
				Model string `json:"model"`
			} `json:"judge_routes"`
		} `json:"judge"`
	}
	if err := readJSON(filepath.Join(root, candidatesFile), &table); err != nil {
		return "", err
	}
	if model := table.Judge.Routes["api"].Model; model != "" {
		return model, nil
	}
	return "", fmt.Errorf("%s names no judge model", candidatesFile)
}

// digestFiles is one digest over several files. Each is framed by its name and
// length, so bytes moved from one file to the next still move the digest.
func digestFiles(root string, names []string) (string, error) {
	sum := sha256.New()
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- a fixed lane file under the root
		if err != nil {
			return "", err
		}
		sum.Write([]byte(name + "\x00" + strconv.Itoa(len(body)) + "\x00"))
		sum.Write(body)
	}
	return short(sum.Sum(nil)), nil
}

// stampOf is one scenario's stamp. A name the catalog does not serve is kept
// as a name, so a tool that appears or disappears still moves the digest.
func (in inputs) stampOf(sc scenario) (Stamp, error) {
	entries := map[string]json.RawMessage{}
	for _, name := range sc.tools {
		served := name
		if router, routed := in.routers[name]; routed {
			if _, plain := in.tools[name]; !plain {
				served = router
			}
		}
		entries[served] = in.tools[served]
		if body, routes := in.catalog[served]; routes {
			entries[catalogEntry+served] = body
		}
	}
	tools, err := json.Marshal(entries)
	if err != nil {
		return Stamp{}, fmt.Errorf("stamping %s: %w", sc.name, err)
	}
	stamp := Stamp{
		Scenario:     digest(sc.body),
		Tools:        digest(tools),
		Instructions: in.instructions,
		Harness:      in.harness,
	}
	if in.door == DoorREST {
		stamp.Tools = in.restTools
	}
	if sc.judged {
		stamp.Judge = in.judge
	}
	return stamp, nil
}

// CurrentStamps is the stamp every scenario would be given if it ran through
// this door on this tree, keyed by scenario name.
func CurrentStamps(root string, door Door) (map[string]Stamp, error) {
	in, err := loadInputs(root, door)
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(root, ScenarioDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no scenario under %s", filepath.Join(root, ScenarioDir))
	}
	out := make(map[string]Stamp, len(files))
	for _, file := range files {
		sc, err := readScenario(file)
		if err != nil {
			return nil, err
		}
		if _, clash := out[sc.name]; clash {
			return nil, fmt.Errorf("two scenarios are named %s; a verdict is filed by name", sc.name)
		}
		if out[sc.name], err = in.stampOf(sc); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Moved names the parts of a recorded stamp that no longer match the current
// one, in a fixed order. Empty means the verdict still describes the product.
func (s Stamp) Moved(current Stamp) []string {
	var moved []string
	for _, part := range []struct {
		name           string
		recorded, want string
	}{
		{"scenario", s.Scenario, current.Scenario},
		{"tool copy", s.Tools, current.Tools},
		{"server instructions", s.Instructions, current.Instructions},
		{"judge", s.Judge, current.Judge},
		{"harness", s.Harness, current.Harness},
	} {
		if part.recorded != part.want {
			moved = append(moved, part.name)
		}
	}
	return moved
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return short(sum[:])
}

// short keeps 16 hex digits: a stamp is compared, never trusted as a proof,
// and a shorter value reads in a diff.
func short(sum []byte) string {
	return hex.EncodeToString(sum)[:16]
}

//craft:ignore naked-any it decodes several committed file shapes, each a caller's own struct
func readJSON(path string, into any) error {
	body, err := os.ReadFile(path) // #nosec G304 -- a committed file of this repository, never a request's
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func marshalIndent(value map[string]Stamp) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

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
	// verb by the router that fronts it.
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

// harnessFiles grade a run or build the world it runs in. A driver is not
// here: how the model was reached is the verdict's folder and `driver`.
var harnessFiles = []string{
	"e2e/llm/check.py", "e2e/llm/judge.py", "e2e/llm/endstate.py",
	"e2e/llm/casesetup.py", "e2e/llm/seed-llm-fixtures.sh",
}

// inputs are the committed files every scenario's stamp is computed from.
type inputs struct {
	tools        map[string]json.RawMessage
	routers      map[string]string
	instructions string
	judge        string
	harness      string
}

func loadInputs(root string) (inputs, error) {
	var info struct {
		Instructions string `json:"instructions"`
		Tools        struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"tools"`
	}
	if err := readJSON(filepath.Join(root, mcpInfoFile), &info); err != nil {
		return inputs{}, err
	}
	if info.Instructions == "" {
		return inputs{}, fmt.Errorf("%s carries no server instructions to stamp", mcpInfoFile)
	}
	in := inputs{tools: map[string]json.RawMessage{}, routers: map[string]string{}, instructions: info.Instructions}
	for _, entry := range info.Tools.Tools {
		// Re-encoded, so the digest reads the entry and not how the file was
		// laid out: the generator's indentation is not tool copy.
		var parsed map[string]any
		if err := json.Unmarshal(entry, &parsed); err != nil {
			return inputs{}, fmt.Errorf("%s: a tool entry: %w", mcpInfoFile, err)
		}
		canonical, err := json.Marshal(parsed)
		if err != nil {
			return inputs{}, fmt.Errorf("%s: a tool entry: %w", mcpInfoFile, err)
		}
		name, named := parsed["name"].(string)
		if !named {
			return inputs{}, fmt.Errorf("%s: a tool entry has no name", mcpInfoFile)
		}
		in.tools[name] = canonical
	}
	if len(in.tools) == 0 {
		return inputs{}, fmt.Errorf("%s lists no tools", mcpInfoFile)
	}
	var routes struct {
		Routers map[string]struct {
			Actions map[string]string `json:"actions"`
		} `json:"routers"`
	}
	if err := readJSON(filepath.Join(root, routesFile), &routes); err != nil {
		return inputs{}, err
	}
	for router, table := range routes.Routers {
		for _, verb := range table.Actions {
			in.routers[verb] = router
		}
	}
	var table struct {
		Judge struct {
			Routes map[string]struct {
				Model string `json:"model"`
			} `json:"judge_routes"`
		} `json:"judge"`
	}
	if err := readJSON(filepath.Join(root, candidatesFile), &table); err != nil {
		return inputs{}, err
	}
	// The canonical judge id is the api route's; e2e/llm/judge.py reads the same.
	if in.judge = table.Judge.Routes["api"].Model; in.judge == "" {
		return inputs{}, fmt.Errorf("%s names no judge model", candidatesFile)
	}
	harness := sha256.New()
	for _, name := range harnessFiles {
		body, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- a fixed harness file under the root
		if err != nil {
			return inputs{}, err
		}
		harness.Write([]byte(name + "\x00" + strconv.Itoa(len(body)) + "\x00"))
		harness.Write(body)
	}
	in.harness = short(harness.Sum(nil))
	return in, nil
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
	}
	tools, err := json.Marshal(entries)
	if err != nil {
		return Stamp{}, fmt.Errorf("stamping %s: %w", sc.name, err)
	}
	stamp := Stamp{
		Scenario:     digest(sc.body),
		Tools:        digest(tools),
		Instructions: digest([]byte(in.instructions)),
		Harness:      in.harness,
	}
	if sc.judged {
		stamp.Judge = in.judge
	}
	return stamp, nil
}

// CurrentStamps is the stamp every scenario would be given if it ran on this
// tree, keyed by scenario name.
func CurrentStamps(root string) (map[string]Stamp, error) {
	in, err := loadInputs(root)
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

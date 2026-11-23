// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agentloope2e judges a scheduled agent that ran on a real seeded
// workspace. The agent is a small multi-turn harness. It explores with real
// tools, thinks, and ends with writes. It can reach the goal several ways, so
// the verdict reads the end state, never the path.
//
// The package holds what needs no database. That is the scenario spec, the
// checks over a snapshot of what a run wrote, the pass bar, the stamp and the
// verdict files. The seeding and the run live in the integration package, which
// hands its snapshot to Check.
package agentloope2e

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Agent names a scheduled agent by the name the runner catalog knows it by.
type Agent string

// The two scheduled agents the lane covers.
const (
	MorningBrief   Agent = "morning_brief"
	OvernightSweep Agent = "overnight_at_risk_sweep"
)

// Scenario is one case: which agent runs, which subjects it must write on and
// which it must never touch. Subjects are logical names from the seed, never
// record ids, because the real writers mint their own.
type Scenario struct {
	Name  string `yaml:"name"`
	Agent Agent  `yaml:"agent"`
	// Worklist is the subjects the agent must write on once each.
	Worklist []string `yaml:"worklist"`
	// Forbidden is the subjects it must not write on: another member's deal,
	// a deal outside the queue, a record a planted instruction names.
	Forbidden []string `yaml:"forbidden"`
	// Canary is a token a planted instruction asks the agent to repeat. It must
	// appear in no write.
	Canary string `yaml:"canary"`
	// NoWrites is a case where the right answer is to write nothing.
	NoWrites bool `yaml:"no_writes"`
	// Criteria are the yes/no questions the judge answers of what the agent
	// wrote, in the form the MCP end-to-end lane's judge takes.
	Criteria []string `yaml:"criteria"`
}

// LoadScenarios reads every *.yaml scenario under dir, in name order, and
// refuses one that cannot be judged or whose name repeats.
func LoadScenarios(dir string) ([]Scenario, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var out []Scenario
	seen := map[string]bool{}
	for _, path := range paths {
		raw, err := os.ReadFile(path) // #nosec G304 -- a scenario file under the lane's own folder
		if err != nil {
			return nil, err
		}
		var sc Scenario
		decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&sc); err != nil {
			return nil, fmt.Errorf("agentloope2e: %s: %w", path, err)
		}
		if err := sc.validate(); err != nil {
			return nil, fmt.Errorf("agentloope2e: %s: %w", path, err)
		}
		if seen[sc.Name] {
			return nil, fmt.Errorf("agentloope2e: two scenarios are named %s; a verdict is filed by name", sc.Name)
		}
		seen[sc.Name] = true
		out = append(out, sc)
	}
	return out, nil
}

func (s Scenario) validate() error {
	switch {
	case s.Name == "":
		return errors.New("the scenario has no name")
	case s.Agent != MorningBrief && s.Agent != OvernightSweep:
		return fmt.Errorf("agent %q is not one the lane covers", s.Agent)
	case s.NoWrites && len(s.Worklist) > 0:
		return errors.New("a scenario that expects no writes names a worklist")
	case !s.NoWrites && len(s.Worklist) == 0:
		return errors.New("a scenario that expects writes names no worklist")
	case len(s.Criteria) == 0 || slices.Contains(s.Criteria, ""):
		return errors.New("the judge needs at least one criterion, and none may be empty")
	}
	return nil
}

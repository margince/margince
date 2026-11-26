// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The ai-tasks.yaml half of the contract to declaration direction, beside
// extjobs.go. Every AI task an extension declares is read back out of the merged
// file, not the fragment. What the boot registers and what the file says cannot
// disagree. The result goes into extensions_gen.go as Go literals.
//
// The rules mirror gen-aitasks: the key prefix, known tiers, queueing and
// degrading modes, the honoured subject restriction, and no payload. `make gen`
// also runs gen-aitasks over the merged file for the rest.

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/pkg/extension"
)

const aiTasksContractBase = "ai-tasks.yaml"

// extAITask is one tasks.<key> entry an extension fragment writes. Strictly
// decoded, so a typo in a key is an error and not a dropped policy.
//
// It is narrower than a core task. An extension writes the whole prompt. So the
// core adds no company context, keeps no payload and honours a restriction of
// processing. Those three are fixed, and the fragment states them for a reviewer.
type extAITask struct {
	DisplayName       string   `yaml:"display_name"`
	Summary           string   `yaml:"summary"`
	Ladder            []string `yaml:"ladder"`
	ExecutionMode     string   `yaml:"execution_mode"`
	OnBudgetExhausted string   `yaml:"on_budget_exhausted"`
	Status            string   `yaml:"status"`
	SubjectRestrict   string   `yaml:"subject_restriction"`
	CompanyContext    string   `yaml:"company_context"`
	NoPayload         bool     `yaml:"no_payload"`
	Sites             []string `yaml:"sites"`
	Doc               string   `yaml:"doc"`
}

// extensionAITasks reads every enabled unit's AI tasks out of the merged
// ai-tasks.yaml. The order is (unit, task), because it is emitted into Go.
func extensionAITasks(units []extensionUnit, contracts map[string][]byte) ([]extension.AITaskDeclaration, error) {
	raw, ok := contracts[aiTasksContractBase]
	if !ok {
		return nil, fmt.Errorf("no composed contract for %s", aiTasksContractBase)
	}
	var doc struct {
		Tiers []string  `yaml:"tiers"`
		Tasks yaml.Node `yaml:"tasks"`
	}
	// Not KnownFields: the core contract carries blocks this reader does not own.
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s/%s: %w", apiLayer, aiTasksContractBase, err)
	}
	owners := make(map[string]string, len(units))
	for _, u := range units {
		ns, err := extension.Name(u.Name).Namespace()
		if err != nil {
			return nil, fmt.Errorf("extensions/%s: %w", u.Name, err)
		}
		owners[ns] = u.Name
	}
	var out []extension.AITaskDeclaration
	if doc.Tasks.IsZero() || doc.Tasks.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(doc.Tasks.Content); i += 2 {
		key := doc.Tasks.Content[i].Value
		if !strings.HasPrefix(key, extension.NamespacePrefix) {
			continue
		}
		decl, err := readExtAITask(key, doc.Tasks.Content[i+1], owners, doc.Tiers)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: task %s: %w", apiLayer, aiTasksContractBase, key, err)
		}
		out = append(out, decl)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Unit != out[j].Unit {
			return out[i].Unit < out[j].Unit
		}
		return out[i].Task < out[j].Task
	})
	return out, nil
}

// readExtAITask decodes one task and holds it to the extension rules.
func readExtAITask(key string, node *yaml.Node, owners map[string]string, tiers []string) (extension.AITaskDeclaration, error) {
	ns := namespaceOf(key, owners)
	unit, ok := owners[ns]
	if !ok {
		return extension.AITaskDeclaration{}, fmt.Errorf("it is in the extension namespace but no enabled unit owns it; a task is named %s_<task> after its unit", "ext_<unit>")
	}
	if !extension.IsShortName(strings.TrimPrefix(key, ns+"_")) {
		return extension.AITaskDeclaration{}, fmt.Errorf("the name after %s_ must be lower snake_case", ns)
	}
	def, err := decodeStrict[extAITask](node)
	if err != nil {
		return extension.AITaskDeclaration{}, err
	}
	if err := checkExtAITask(def, tiers); err != nil {
		return extension.AITaskDeclaration{}, err
	}
	return extension.AITaskDeclaration{
		Unit: extension.Name(unit), Task: key, DisplayName: def.DisplayName, Summary: def.Summary,
		Ladder: def.Ladder, Background: def.ExecutionMode == "background", Sites: def.Sites,
	}, nil
}

// checkExtAITask holds a task to the gen-aitasks rules an extension can meet.
func checkExtAITask(def extAITask, tiers []string) error {
	if strings.TrimSpace(def.DisplayName) == "" || strings.TrimSpace(def.Summary) == "" {
		return errors.New("it needs a display_name and a summary, which Settings shows")
	}
	if len(def.Ladder) == 0 {
		return errors.New("its ladder is empty")
	}
	for _, tier := range def.Ladder {
		if !slices.Contains(tiers, tier) {
			return fmt.Errorf("its ladder names unknown tier %q", tier)
		}
	}
	switch {
	case def.ExecutionMode == "interactive" && def.OnBudgetExhausted == "degrade":
	case def.ExecutionMode == "background" && def.OnBudgetExhausted == "queue":
	case def.ExecutionMode != "interactive" && def.ExecutionMode != "background":
		return fmt.Errorf("execution_mode must be interactive or background, not %q", def.ExecutionMode)
	default:
		return fmt.Errorf("%s execution_mode needs on_budget_exhausted %q, not %q: interactive tasks degrade and background tasks queue",
			def.ExecutionMode, map[string]string{"interactive": "degrade", "background": "queue"}[def.ExecutionMode], def.OnBudgetExhausted)
	}
	if def.Status != "shipped" || len(def.Sites) == 0 {
		return errors.New("it must be status shipped with at least one site")
	}
	if def.SubjectRestrict != "honour" {
		return errors.New("subject_restriction must be honour: the core cannot see inside a prompt the unit wrote")
	}
	if !def.NoPayload {
		return errors.New("no_payload must be true: the core keeps no payload of a prompt the unit wrote")
	}
	if def.CompanyContext != "none" {
		return errors.New("company_context must be none: the unit writes the whole prompt")
	}
	return nil
}

// writeAITaskLiterals emits AITasks() beside Jobs() and Verbs().
func writeAITaskLiterals(b *strings.Builder, decls []extension.AITaskDeclaration) {
	if len(decls) == 0 {
		b.WriteString("\treturn nil\n}\n")
		return
	}
	b.WriteString("\treturn []extension.AITaskDeclaration{\n")
	for _, d := range decls {
		b.WriteString("\t\t{\n")
		fmt.Fprintf(b, "\t\t\tUnit:        %q,\n", string(d.Unit))
		fmt.Fprintf(b, "\t\t\tTask:        %q,\n", d.Task)
		fmt.Fprintf(b, "\t\t\tDisplayName: %q,\n", d.DisplayName)
		fmt.Fprintf(b, "\t\t\tSummary:     %q,\n", d.Summary)
		fmt.Fprintf(b, "\t\t\tLadder:      %#v,\n", d.Ladder)
		fmt.Fprintf(b, "\t\t\tBackground:  %t,\n", d.Background)
		fmt.Fprintf(b, "\t\t\tSites:       %#v,\n", d.Sites)
		b.WriteString("\t\t},\n")
	}
	b.WriteString("\t}\n}\n")
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// goodTask is one well-formed task body, indented as an overlay update.
const goodTask = `      display_name: "Order summary"
      summary: "Writes a one-line summary of an order."
      ladder: [alpha]
      execution_mode: interactive
      on_budget_exhausted: degrade
      status: shipped
      subject_restriction: honour
      company_context: none
      no_payload: true
      sites: [summarise]
`

// composeTasks composes one unit's ai-tasks.yaml fragment against the stand-in
// base and reads the extension tasks back out of the merged document.
func composeTasks(t *testing.T, target, body string) (int, error) {
	t.Helper()
	root := fragmentRoot(t, map[string]map[string]string{
		"demo": {"ai-tasks.yaml": overlayFor("demo", target, body)},
	})
	contracts, err := composeFrom(t, root)
	if err != nil {
		return 0, err
	}
	units, err := scanExtensions(root)
	if err != nil {
		return 0, err
	}
	decls, err := extensionAITasks(units, contracts)
	return len(decls), err
}

func TestExtensionAITasksReadsATaskOutOfTheMergedContract(t *testing.T) {
	root := fragmentRoot(t, map[string]map[string]string{
		"demo": {"ai-tasks.yaml": overlayFor("demo", "$.tasks['ext_demo_summarise']", goodTask)},
	})
	contracts, err := composeFrom(t, root)
	if err != nil {
		t.Fatal(err)
	}
	units, err := scanExtensions(root)
	if err != nil {
		t.Fatal(err)
	}
	decls, err := extensionAITasks(units, contracts)
	if err != nil || len(decls) != 1 {
		t.Fatalf("decls, err = %v, %v", decls, err)
	}
	d := decls[0]
	if d.Unit != "demo" || d.Task != "ext_demo_summarise" || d.Background || d.DisplayName != "Order summary" || len(d.Ladder) != 1 {
		t.Fatalf("task came back as %+v", d)
	}
	var b strings.Builder
	writeAITaskLiterals(&b, decls)
	if !strings.Contains(b.String(), `Task:        "ext_demo_summarise"`) {
		t.Fatalf("literal is missing the task:\n%s", b.String())
	}
}

func TestExtensionAITasksReadsABackgroundTask(t *testing.T) {
	body := strings.NewReplacer("interactive", "background", "degrade", "queue").Replace(goodTask)
	root := fragmentRoot(t, map[string]map[string]string{
		"demo": {"ai-tasks.yaml": overlayFor("demo", "$.tasks['ext_demo_nightly']", body)},
	})
	contracts, err := composeFrom(t, root)
	if err != nil {
		t.Fatal(err)
	}
	units, _ := scanExtensions(root)
	decls, err := extensionAITasks(units, contracts)
	if err != nil || len(decls) != 1 || !decls[0].Background {
		t.Fatalf("decls, err = %+v, %v", decls, err)
	}
}

func TestExtensionAITasksRefusesWhatGenerationMustNotPublish(t *testing.T) {
	for _, tc := range []struct {
		name, target, body, want string
	}{
		{"no unit prefix", "$.tasks['summarise']", goodTask, "outside the unit's task namespace"},
		{"another unit's prefix", "$.tasks['ext_other_summarise']", goodTask, "outside the unit's task namespace"},
		{"unknown tier", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "[alpha]", "[huge]", 1), `unknown tier "huge"`},
		{"interactive that queues", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "degrade", "queue", 1), "interactive tasks degrade"},
		{"background that degrades", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "interactive", "background", 1), "background tasks queue"},
		{"unknown mode", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "interactive", "eventually", 1), "execution_mode must be"},
		{"restriction not honoured", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "honour", "none", 1), "subject_restriction must be honour"},
		{"payload kept", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "no_payload: true", "no_payload: false", 1), "no_payload must be true"},
		{"company context", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "company_context: none", "company_context: {scopes: [offer]}", 1), "cannot unmarshal !!map into string"},
		{"no site", "$.tasks['ext_demo_a']", strings.Replace(goodTask, "sites: [summarise]", "sites: []", 1), "at least one site"},
		{"a typo'd key", "$.tasks['ext_demo_a']", goodTask + "      budget: lots\n", "field budget not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := composeTasks(t, tc.target, tc.body)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestExtensionAITasksIgnoresTheCoreTasks(t *testing.T) {
	decls, err := extensionAITasks(nil, map[string][]byte{aiTasksContractBase: []byte("tiers: [alpha]\ntasks:\n  core:\n    ladder: [alpha]\n")})
	if err != nil || len(decls) != 0 {
		t.Fatalf("decls, err = %v, %v", decls, err)
	}
}

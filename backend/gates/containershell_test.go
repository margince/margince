// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every workflow job that runs in a container names bash as its steps' shell.
//
// On the runner a step with no `shell:` runs bash, and inside a `container:`
// it runs `sh`. The steps here use `set -o pipefail` and bash arrays, which
// `sh` refuses on the first line, so the job fails before any check runs.

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// shellDefaults is the `defaults.run` block, at workflow or job level. Run is
// nil when the block is absent, so absent tells apart from a block with no shell.
type shellDefaults struct {
	Run *struct {
		Shell string `yaml:"shell"`
	} `yaml:"run"`
}

// containerShell is the shell a job's steps get: a job's own `defaults.run`
// replaces the workflow's whole block, so a job that sets only
// `working-directory` there runs `sh`.
func containerShell(workflow, job shellDefaults) string {
	if job.Run != nil {
		return job.Run.Shell
	}
	if workflow.Run != nil {
		return workflow.Run.Shell
	}
	return ""
}

type containerWorkflow struct {
	Defaults shellDefaults `yaml:"defaults"`
	Jobs     map[string]struct {
		Container yaml.Node     `yaml:"container"`
		Defaults  shellDefaults `yaml:"defaults"`
	} `yaml:"jobs"`
}

func TestEveryContainerJobRunsItsStepsInBash(t *testing.T) {
	t.Parallel()
	containerJobs := 0
	for _, path := range workflowFiles(t) {
		raw, err := gatekit.ReadFile(path) // #nosec G304 -- a repo-relative workflow path from the glob
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		var wf containerWorkflow
		if err := unmarshalYAML(raw, &wf); err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for name, job := range wf.Jobs {
			if job.Container.IsZero() {
				continue
			}
			containerJobs++
			if shell := containerShell(wf.Defaults, job.Defaults); shell != "bash" {
				t.Errorf("%s: job %q runs in a container with default shell %q; set `defaults.run.shell: bash` on the job, or on the workflow when the job has no `defaults.run` of its own",
					path, name, shell)
			}
		}
	}
	// The CI image runs jobs in several workflows, so a census of none read nothing.
	if containerJobs == 0 {
		t.Fatal("no workflow job declares a container; this gate read nothing")
	}
}

func TestAJobsOwnRunDefaultsReplaceTheWorkflowsShell(t *testing.T) {
	t.Parallel()
	const workflow = `
defaults:
  run:
    shell: bash
jobs:
  inherits:
    container: image
  replaces:
    container: image
    defaults:
      run:
        working-directory: .
  restates:
    container: image
    defaults:
      run:
        shell: bash
        working-directory: .
`
	var wf containerWorkflow
	if err := unmarshalYAML([]byte(workflow), &wf); err != nil {
		t.Fatalf("parsing the planted workflow: %v", err)
	}
	want := map[string]string{"inherits": "bash", "replaces": "", "restates": "bash"}
	for name, shell := range want {
		if got := containerShell(wf.Defaults, wf.Jobs[name].Defaults); got != shell {
			t.Errorf("job %q: shell %q, want %q", name, got, shell)
		}
	}
}

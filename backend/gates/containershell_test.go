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
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// shellDefaults is the `defaults.run.shell` key, at workflow or job level.
type shellDefaults struct {
	Run struct {
		Shell string `yaml:"shell"`
	} `yaml:"run"`
}

type containerWorkflow struct {
	Defaults shellDefaults `yaml:"defaults"`
	Jobs     map[string]struct {
		Container yaml.Node     `yaml:"container"`
		Defaults  shellDefaults `yaml:"defaults"`
	} `yaml:"jobs"`
}

func TestEveryContainerJobRunsItsStepsInBash(t *testing.T) {
	containerJobs := 0
	for _, path := range workflowFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		var wf containerWorkflow
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for name, job := range wf.Jobs {
			if job.Container.IsZero() {
				continue
			}
			containerJobs++
			shell := job.Defaults.Run.Shell
			if shell == "" {
				shell = wf.Defaults.Run.Shell
			}
			if shell != "bash" {
				t.Errorf("%s: job %q runs in a container with default shell %q; set `defaults.run.shell: bash` on the workflow or the job",
					path, name, shell)
			}
		}
	}
	// The CI image runs jobs in several workflows, so a census of none read nothing.
	if containerJobs == 0 {
		t.Fatal("no workflow job declares a container; this gate read nothing")
	}
}

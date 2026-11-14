// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"path/filepath"
	"regexp"
	"testing"
)

func TestReleaseWorkflowSelectsPublisherReleasesWithReleaseOption(t *testing.T) {
	t.Parallel()
	wf := readWorkflowJobs(t, filepath.Join(workflowDir, "release.yml"))
	commands := map[string]string{
		"resolve-patch-base": `--release[[:space:]]+"\$version"`,
		"draft-release":      `--release[[:space:]]+"\$version"`,
		"add-artifacts":      `--release[[:space:]]+"\$VERSION"`,
		"publish-release":    `--release[[:space:]]+"\$VERSION"`,
	}
	for command, pattern := range commands {
		commandPattern := regexp.MustCompile(`\b` + command + `\b`)
		selectorPattern := regexp.MustCompile(`\b` + command + `\b[[:space:]]+` + pattern)
		calls, selected := 0, 0
		for _, job := range wf.Jobs {
			for _, step := range job.Steps {
				calls += len(commandPattern.FindAllString(step.Run, -1))
				selected += len(selectorPattern.FindAllString(step.Run, -1))
			}
		}
		if calls == 0 {
			t.Errorf("release.yml does not run %s; this check must cover every publisher release selector", command)
		}
		if selected != calls {
			t.Errorf("release.yml selects %d of %d %s calls with --release", selected, calls, command)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// Registration probes compile the real generated constraint through a source overlay.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// jobKindProbeSource is one extra file for internal/compose that registers a
// single kind through the sanctioned door. The args type is the parameter: a
// DECLARED one must compile, and the undeclared one beside it must not.
//
// The unused type in the declared case is deliberate — one source, one
// difference, so the two builds differ in the type argument and in nothing
// else that could explain a refusal.
const jobKindProbeSource = `package compose

import (
	"context"

	"github.com/riverqueue/river"
)

type undeclaredProbeArgs struct{}

func (undeclaredProbeArgs) Kind() string { return "undeclared_probe" }

type jobKindProbeWorker struct{}

func (jobKindProbeWorker) Work(context.Context, *river.Job[%[2]s]) error { return nil }

func registerJobKindProbe(reg *jobRegistry) {
	addDeclaredWorker[%[1]s](reg, jobKindProbeWorker{})
}
`

// buildComposeWith compiles the real internal/compose package with one extra
// source file layered over it, and reports the compiler's own output. The
// overlay is go build's own mechanism: the file exists for the duration of the
// build and never lands in the tree, so this gate cannot leave a probe kind
// behind for another lane to register.
//
// An empty body builds the package as it stands, which is the control: a
// refusal below means nothing unless the same package compiles without the
// probe.
func buildComposeWith(t *testing.T, body string) (string, bool) {
	t.Helper()
	args := []string{"build", "./internal/compose"}
	if body != "" {
		dir := t.TempDir()
		probe := filepath.Join(dir, "probe.go")
		if err := os.WriteFile(probe, []byte(body), 0o600); err != nil {
			t.Fatalf("writing the probe source: %v", err)
		}
		target, err := filepath.Abs(filepath.Join("internal", "compose", "zz_jobkindprobe.go"))
		if err != nil {
			t.Fatalf("resolving the overlay target: %v", err)
		}
		overlay := filepath.Join(dir, "overlay.json")
		spec, err := json.Marshal(map[string]map[string]string{"Replace": {target: probe}})
		if err != nil {
			t.Fatalf("encoding the overlay: %v", err)
		}
		if err := os.WriteFile(overlay, spec, 0o600); err != nil {
			t.Fatalf("writing the overlay: %v", err)
		}
		args = []string{"build", "-overlay=" + overlay, "./internal/compose"}
	}
	cmd := exec.Command("go", args...)
	// The environment is inherited, unlike the scratch module above: this build
	// is of the real package in the real module, and the workspace the lane
	// runs under is the one it must resolve through.
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

func TestGeneratedRegistrationChecksTheArgsAndWorkerTypes(t *testing.T) {
	t.Parallel()
	if out, ok := buildComposeWith(t, ""); !ok {
		t.Fatalf("compose control does not build:\n%s", out)
	}
	for _, probe := range []struct {
		args, worker string
		accepted     bool
	}{
		{"VoiceBuildArgs", "VoiceBuildArgs", true},
		{"undeclaredProbeArgs", "undeclaredProbeArgs", false},
		{"VoiceBuildArgs", "SendEmailArgs", false},
	} {
		source := fmt.Sprintf(jobKindProbeSource, probe.args, probe.worker)
		out, ok := buildComposeWith(t, source)
		if ok != probe.accepted {
			t.Errorf("args=%s worker=%s: accepted=%v:\n%s", probe.args, probe.worker, ok, out)
		}
		if !ok {
			expected := "jobKindProbeWorker"
			if probe.args == "undeclaredProbeArgs" {
				expected = "does not satisfy declaredJobArgs[undeclaredProbeArgs]"
			}
			if !strings.Contains(out, expected) {
				t.Errorf("refused outside registration (%s): %s", expected, out)
			}
		}
	}
}

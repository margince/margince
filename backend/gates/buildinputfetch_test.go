// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A build input fetched over the network survives a transient failure.
//
// Nothing this repository builds with is authored at build time: the Go modules
// come from a proxy, the craftsmanship gate and the secret scanner come from a
// pinned release. Each is content-checked — go.sum and the checksum database
// for a module, a SHA-256 pin for a tool — so WHERE the bytes come from and how
// many attempts it took cannot change what is accepted. Only whether the fetch
// happened at all.
//
// A fetch that gives up on the first bad minute does not read as a bad minute.
// It reads as a red tree. Both halves of this have already happened, on the
// same day: a `stream error ... INTERNAL_ERROR` on one module surfaced inside a
// test shard as `FAIL [setup failed]`, and the main-health check reported a red
// `main` and named fifteen recent commits as the likely cause — none of them
// related. A 504 from the release host failed the craftsmanship gate on a pull
// request whose diff was clean. In both cases every author downstream inherits
// the failure and reads it as something they broke, and the real cause is
// invisible unless somebody opens the raw log.
//
// So the obligation is one thing said in two dialects, and it is spelled here
// once rather than in each: a network fetch of a build input retries or falls
// through, and a digest still decides what is accepted.
//
// WHAT IT CANNOT SEE. A fetch written in some third way — a workflow step that
// curls a tool inline, a Go job that runs without actions/setup-go. Neither
// exists today, and either would be a larger change than a forgotten flag.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// goProxyFallThrough is the value that falls through on any proxy error. Go's
// default `https://proxy.golang.org,direct` falls through only on a 404 or 410
// — every other failure, a 5xx or a dropped stream included, is fatal to the
// command. The PIPE is the whole point; a comma here restores the outage.
const goProxyFallThrough = "https://proxy.golang.org|direct"

// goToolchainAction is what marks a workflow as running Go. Matched on the
// action path without its version, so a pinned-SHA bump does not blind the gate.
const goToolchainAction = "actions/setup-go@"

// pinScriptGlob matches the scripts that download a pinned tool. Derived from
// the naming convention rather than listed, so a third pinned tool is covered
// the day its script is committed.
const pinScriptGlob = "../scripts/*-pin.sh"

// workflowEnv is the shape this gate needs: the workflow-level env block, which
// every job in the file inherits.
type workflowEnv struct {
	Env map[string]string `yaml:"env"`
}

func TestEveryGoWorkflowFallsThroughAFailingModuleProxy(t *testing.T) {
	t.Parallel()
	files := workflowFiles(t)

	running := 0
	for _, path := range files {
		raw, err := os.ReadFile(path) // #nosec G304 -- a repo-relative workflow path from the glob above
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if !strings.Contains(string(raw), goToolchainAction) {
			continue
		}
		running++
		var wf workflowEnv
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got := wf.Env["GOPROXY"]; got != goProxyFallThrough {
			t.Errorf("%s installs a Go toolchain and declares GOPROXY=%q, want %q at workflow level.\n\n"+
				"GitHub Actions has no include, so the line is repeated per workflow and this is what "+
				"keeps that true. Without it a 5xx from the module mirror fails the command outright, and "+
				"inside a test shard that reads as a broken product",
				filepath.Base(path), got, goProxyFallThrough)
		}
	}
	// The census that can fail short: an action rename would leave every
	// workflow unexamined and read exactly like a clean tree.
	if running == 0 {
		t.Errorf("no workflow under %s installs a Go toolchain — %q matched nothing, so this gate judged "+
			"no file at all; correct the marker rather than leaving it reading green over an empty sweep",
			workflowDir, goToolchainAction)
	}
}

func TestEveryPinnedToolDownloadRetries(t *testing.T) {
	t.Parallel()
	scripts, err := filepath.Glob(pinScriptGlob)
	if err != nil {
		t.Fatalf("listing pin scripts: %v", err)
	}
	if len(scripts) == 0 {
		t.Fatalf("no pin scripts matched %s; this gate would pass vacuously", pinScriptGlob)
	}
	for _, path := range scripts {
		raw, err := os.ReadFile(path) // #nosec G304 -- a repo-relative script path from the glob above
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.Contains(line, "curl ") || strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if !strings.Contains(line, "--retry ") {
				t.Errorf("%s downloads with a curl that does not retry:\n\t%s\n\n"+
					"The release host answers 5xx often enough to have reported a red gate against an "+
					"unchanged tree. Retrying changes nothing about trust — the digest check below still "+
					"decides what is accepted, and it is fatal on the first try",
					filepath.Base(path), strings.TrimSpace(line))
			}
		}
	}
}

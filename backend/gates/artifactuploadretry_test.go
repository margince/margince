// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Every artifact upload goes through the wrapper that retries it.
//
// One 403 from GitHub's artifact service used to cost a whole lane: the shard's
// coverage pods never arrive, the fan-in cannot merge them, `ci` reports the
// fan-in's failure, and the push-time check files a "main is red" issue against
// a tree whose every test passed. The wrapper tries twice — loudly on the
// second — so a transient stops costing that and a genuine loss still does.
//
// A prohibition rather than a census: it is the DIRECT call that reintroduces
// the defect, and the next one will be pasted from a workflow that predates
// this. The corpus is the workflow tree, so a workflow added later is covered
// the day it is committed.

import (
	"os"
	"strings"
	"testing"
)

// retryingUploadAction is the one door. Its own definition names
// actions/upload-artifact twice, which is exactly why this gate reads the
// workflow tree and not the whole repository.
const retryingUploadAction = "./.github/actions/upload-artifact-retried"

// directUploadAction is what a workflow must not say. Matched without its
// version pin: the defect is calling the action from a workflow at all, not
// calling one version of it.
const directUploadAction = "uses: actions/upload-artifact@"

func TestEveryWorkflowUploadsItsArtifactsThroughTheRetryingWrapper(t *testing.T) {
	t.Parallel()
	wrapped := 0
	for _, path := range workflowFiles(t) {
		raw, err := os.ReadFile(path) // #nosec G304 -- a repo-relative workflow path from the glob
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			switch {
			case strings.Contains(line, directUploadAction):
				t.Errorf("%s:%d calls actions/upload-artifact directly. Use %s, which tries twice: "+
					"a transient artifact-service failure here fails the whole lane, and the "+
					"push-time check files a main-red issue against a tree that passes.",
					path, i+1, retryingUploadAction)
			case strings.Contains(line, retryingUploadAction):
				wrapped++
			}
		}
	}
	// Under-recognition is the one way this must not break: a scan that matched
	// nothing would report PASS while every upload in the tree was direct.
	if wrapped == 0 {
		t.Fatal("no workflow step uses " + retryingUploadAction +
			"; either the wrapper is gone or this scan no longer sees its own subject")
	}
}

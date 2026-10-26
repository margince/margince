// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/datatask"
)

func TestTheVerdictNamesEveryStepThatDidNotFinish(t *testing.T) {
	verdict := DataTaskVerdictOf([]datatask.Progress{
		{TaskID: "core/1_done", Step: "a", Status: datatask.Done, Scanned: 3},
		{TaskID: "core/2_empty", Step: "a", Status: datatask.Done},
		{TaskID: "core/3_review", Step: "a", Status: datatask.Pending, BlockedReason: datatask.ReviewRequired},
		{TaskID: "core/4_failed", Step: "a", Status: datatask.Failed, BlockedReason: "earlier_step"},
		{TaskID: "core/5_held", Step: "a", Status: datatask.Pending, BlockedReason: "missing_prerequisite"},
		{TaskID: "core/6_pending", Step: "a", Status: datatask.Pending},
	})
	if verdict.Finished || verdict.Done != 2 {
		t.Errorf("verdict = %+v, want two done and not finished", verdict)
	}
	for name, c := range map[string]struct{ got, want []string }{
		"nothing inspected": {verdict.NothingInspected, []string{"core/2_empty/a"}},
		"awaiting review":   {verdict.AwaitingReview, []string{"core/3_review/a"}},
		"failed":            {verdict.Failed, []string{"core/4_failed/a (failed)"}},
		"blocked":           {verdict.Blocked, []string{"core/5_held/a (missing_prerequisite)"}},
		"unfinished":        {verdict.Unfinished, []string{"core/6_pending/a"}},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s = %v, want %v", name, c.got, c.want)
		}
	}
	if all := DataTaskVerdictOf([]datatask.Progress{{TaskID: "core/1", Step: "a", Status: datatask.Done, Scanned: 1}}); !all.Finished {
		t.Errorf("a run with every step done is not finished: %+v", all)
	}
}

// data-tasks execute promises that no correction reaches a mailbox, a model or
// a vendor. A correction file that imported a client for one would break it.
func TestNoDataTaskImportsAnOutboundClient(t *testing.T) {
	files, err := filepath.Glob("datatask_*.go")
	if err != nil || len(files) < 20 {
		t.Fatalf("found %d data task files (err %v); the census lost its subject", len(files), err)
	}
	// A provider package may be read for its parsers, as two repairs read stored
	// calendar originals. Building its client is what would reach the vendor.
	forbidden := []string{"net/http", "github.com/riverqueue/", "/platform/jobs", "/modules/ai", "/enrich"}
	clients := []string{"NewAPI(", "NewOAuth(", "NewClient("}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range clients {
			if strings.Contains(string(source), c) {
				t.Errorf("%s builds a provider client with %s; a data task must stay inside the database", file, c)
			}
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, f := range forbidden {
				if strings.Contains(path, f) {
					t.Errorf("%s imports %s; a data task must stay inside the database", file, path)
				}
			}
		}
	}
}

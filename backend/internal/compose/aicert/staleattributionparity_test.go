// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The summary's stale count is every record the JSON attributes, deduplicated —
// never a subset of them.
//
// Under-reporting is the one direction a certification summary must not fail in:
// a figure that reads lower than the truth is one nobody re-runs, and nothing
// about it looks wrong. The summary counts per (task, binding) while the JSON
// carries one entry per site a task ships, so the two figures differ by design
// and a reader comparing them cannot tell that from an undercount — which is
// what this asks of the committed pair rather than of the renderer that wrote
// both.
//
// Read off the FILES, not recomputed from the same in-memory document: a bug in
// the narrowing would move both halves of a check that shared the walk, and
// agree with itself.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// staleSummarySentence pulls the two figures the summary prints: the records it
// attributes, and the JSON entries it says those records came from.
var staleSummarySentence = regexp.MustCompile(
	`over the (\d+) stale record\(s\) this build can attribute\..*?its (\d+) ` + "`stale_cause`" + ` entries are these (\d+) records`)

// certPair is the two generated files, as this check reads them.
const (
	certJSONPath = "../../../../docs/reference/ai-certification.json"
	certMDPath   = "../../../../docs/reference/ai-certification.md"
)

func TestTheStaleSummaryCountsEveryRecordTheJSONAttributes(t *testing.T) {
	t.Parallel()
	page, err := os.ReadFile(filepath.Clean(certMDPath))
	if err != nil {
		t.Fatalf("reading the certification page: %v", err)
	}
	found := staleSummarySentence.FindStringSubmatch(string(page))
	if found == nil {
		if strings.Contains(string(page), "stale record(s) this build can attribute") {
			t.Fatal("the stale summary no longer states how many JSON entries its records came from, so a " +
				"reader comparing the two files cannot tell the dedupe from an undercount")
		}
		t.Skip("this build attributed no stale record, so the summary is absent by design")
	}

	attributed, rows := mustAtoi(t, found[1]), mustAtoi(t, found[2])
	if restated := mustAtoi(t, found[3]); restated != attributed {
		t.Errorf("the summary says %d records and then %d, in one sentence", attributed, restated)
	}

	jsonRows, jsonRecords := staleCauseCounts(t)
	if rows != jsonRows {
		t.Errorf("the summary says the JSON carries %d stale_cause entries and it carries %d — "+
			"the pair is regenerated together, so one of the two walks has narrowed", rows, jsonRows)
	}
	if attributed != jsonRecords {
		t.Errorf("the summary attributes %d record(s) and the JSON holds %d distinct (task, binding) "+
			"pair(s) with a cause. Lower here understates stale certification, which is the direction "+
			"nobody re-runs on.", attributed, jsonRecords)
	}
}

// staleCauseCounts reads the JSON half: how many records carry a cause, and how
// many distinct (task, binding) pairs those are.
func staleCauseCounts(t *testing.T) (rows, records int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(certJSONPath))
	if err != nil {
		t.Fatalf("reading the certification JSON: %v", err)
	}
	var doc struct {
		Sites []struct {
			Task    string `json:"task"`
			Records []struct {
				Binding struct {
					Provider string `json:"provider"`
					Model    string `json:"model"`
					Env      string `json:"env"`
				} `json:"binding"`
				StaleCause *struct{} `json:"stale_cause"`
			} `json:"records"`
		} `json:"sites"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the certification JSON: %v", err)
	}
	distinct := map[string]bool{}
	for _, site := range doc.Sites {
		for _, rec := range site.Records {
			if rec.StaleCause == nil {
				continue
			}
			rows++
			distinct[strings.Join([]string{site.Task, rec.Binding.Provider, rec.Binding.Model, rec.Binding.Env}, "\x00")] = true
		}
	}
	return rows, len(distinct)
}

func mustAtoi(t *testing.T, text string) int {
	t.Helper()
	n, err := strconv.Atoi(text)
	if err != nil {
		t.Fatalf("reading %q as a count: %v", text, err)
	}
	return n
}

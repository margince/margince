// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// The skill bundle's guides are Markdown pages a user reads, so they meet the
// house prose bar and cite nothing private. Neither tree gate reads them: they
// are templates (*.md.tmpl) until a download renders them. This renders them
// through the production builder and runs both gates' own checks on the result.

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentbundle"
)

func TestTheSkillBundleReadsLikeTheDocs(t *testing.T) {
	t.Parallel()
	archive, err := (&agentbundle.Builder{}).Build("https://crm.example.test/v1")
	if err != nil {
		t.Fatalf("building the skill bundle: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("the skill bundle is not a ZIP: %v", err)
	}
	guides := 0
	for _, f := range reader.File {
		body := readZipFile(t, f)
		for i, line := range strings.Split(body, "\n") {
			for _, rule := range forbidden {
				if rule.pattern.MatchString(line) {
					t.Errorf("%s:%d carries a %s: %s", f.Name, i+1, rule.name, rule.why)
				}
			}
		}
		// The index and the contract are generated from crm.yaml; the guides
		// are the prose written for a reader.
		if !strings.HasSuffix(f.Name, "/README.md") && !strings.HasSuffix(f.Name, "/SKILL.md") {
			continue
		}
		guides++
		for _, v := range checkHouseBar(f.Name, body, false) {
			t.Errorf("%s:%d: [%s] %s", f.Name, v.line, v.rule, v.what)
		}
	}
	if guides != 2 {
		t.Fatalf("read %d guides from the bundle, want README.md and SKILL.md", guides)
	}
}

func readZipFile(t *testing.T, f *zip.File) string {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatalf("opening %s: %v", f.Name, err)
	}
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading %s: %v", f.Name, err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("closing %s: %v", f.Name, err)
	}
	return string(body)
}

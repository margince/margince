// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestFetchPageKeepsHeadingSectionsWithoutChangingText(t *testing.T) {
	doc := `<html><body><h1>Imprint</h1><h2>Acme Singapore</h2>
	<p>Acme Pte. Ltd.</p><p>77 High Street, Singapore (179433)</p>
	<p>Business Profile: 201629357M</p><h2>Acme Thailand</h2>
	<p>Acme (Thailand) Co., Ltd.</p><p>7 Summer Road, Bangkok</p>
	<script>const hidden = "<h2>Invented company</h2>";</script></body></html>`
	page, err := testFetcher().FetchPage(context.Background(), serveHTML(t, doc).URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Imprint", "Acme Singapore Acme Pte. Ltd. 77 High Street, Singapore (179433) Business Profile: 201629357M",
		"Acme Thailand Acme (Thailand) Co., Ltd. 7 Summer Road, Bangkok",
	}
	if !slices.Equal(page.Sections, want) {
		t.Fatalf("sections = %#v, want %#v", page.Sections, want)
	}
	if page.Text != StripTags(doc) {
		t.Fatal("heading evidence changed the existing page text contract")
	}
	for _, section := range page.Sections {
		if !strings.Contains(page.Text, section) {
			t.Errorf("section is not verbatim page text: %q", section)
		}
	}
}

func TestAHeadinglessPageKeepsTheUnstructuredEvidencePath(t *testing.T) {
	if sections := HeadingSections(`<p>Acme GmbH</p><p>First Street 1</p>`); sections != nil {
		t.Fatalf("paragraphs must not pretend to identify company boundaries: %#v", sections)
	}
}

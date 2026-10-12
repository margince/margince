// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compliancedocs

import (
	"strings"
	"testing"
)

func validDoc() Document {
	return Document{
		Jurisdiction: "zz", ID: "staff-notice", Title: "Staff notice", Language: "en",
		Audience: Employee, Kind: Template, Body: []byte("# Notice"), MediaType: "text/markdown",
	}
}

func TestADocumentTheAdminPageCannotServeIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(*Document)
		want   string
	}{
		{"empty body", func(d *Document) { d.Body = nil }, "empty"},
		{"unknown media type", func(d *Document) { d.MediaType = "application/zip" }, "media type"},
		{"bad id", func(d *Document) { d.ID = "Staff Notice" }, "lower-case"},
		{"unknown audience", func(d *Document) { d.Audience = "auditor" }, "audience"},
		{"unknown kind", func(d *Document) { d.Kind = "draft" }, "kind"},
		{"no title", func(d *Document) { d.Title = " " }, "no title"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := validDoc()
			c.mutate(&d)
			if err := ValidateSet([]Document{d}); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ValidateSet = %v, want an error containing %q", err, c.want)
			}
		})
	}
	if err := ValidateSet([]Document{validDoc()}); err != nil {
		t.Fatalf("a valid document is refused: %v", err)
	}
}

func TestTwoDocumentsUnderOneIDAreRefused(t *testing.T) {
	err := ValidateSet([]Document{validDoc(), validDoc()})
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("ValidateSet = %v, want the duplicate refused", err)
	}
	other := validDoc()
	other.Jurisdiction = "zy"
	if err := ValidateSet([]Document{validDoc(), other}); err != nil {
		t.Fatalf("one ID under two jurisdictions is refused: %v", err)
	}
}

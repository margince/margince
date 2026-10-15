// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// The compiled-in ceilings, written out here. This is the test that notices a
// default moving; one that read the default it checks would agree with any
// value.
var compiledCeilings = UploadLimits{
	Attachment: 25_000_000, CSVImport: 10_000_000, LinkedInImport: 8_000_000, KnowledgeDocument: 5_000_000,
}

func loadUploads(t *testing.T, body string) (UploadLimits, error) {
	t.Helper()
	cfg, err := configtest.Load(t, "version: 1\n"+body, nil, nil, UploadSettings()...)
	if err != nil {
		return UploadLimits{}, err
	}
	return UploadLimitsFrom(cfg), nil
}

// An installation that never wrote the section is still bounded, generously
// enough to upload the paper the product is about. A composition with no file
// at all uses the same numbers.
func TestAFileWithNoUploadsSectionGetsTheCompiledCeilings(t *testing.T) {
	limits, err := loadUploads(t, "")
	if err != nil || limits != compiledCeilings {
		t.Fatalf("unconfigured limits %+v (%v), want %+v", limits, err, compiledCeilings)
	}
	if DefaultUploadLimits() != compiledCeilings {
		t.Errorf("DefaultUploadLimits() = %+v, want %+v", DefaultUploadLimits(), compiledCeilings)
	}
}

// Naming one route must not zero the others: a zero ceiling refuses every
// upload on a route nobody was thinking about.
func TestOneConfiguredCeilingLeavesTheOthersAlone(t *testing.T) {
	limits, err := loadUploads(t, "uploads:\n  attachment_mb: 40\n")
	want := compiledCeilings
	want.Attachment = 40_000_000
	if err != nil || limits != want {
		t.Fatalf("limits %+v (%v), want %+v", limits, err, want)
	}
}

func TestEveryRouteIsSeparatelyConfigurable(t *testing.T) {
	limits, err := loadUploads(t, "uploads:\n  attachment_mb: 50\n  csv_import_mb: 20\n  linkedin_import_mb: 3\n  knowledge_document_mb: 7\n")
	want := UploadLimits{Attachment: 50_000_000, CSVImport: 20_000_000, LinkedInImport: 3_000_000, KnowledgeDocument: 7_000_000}
	if err != nil || limits != want {
		t.Fatalf("limits %+v (%v), want %+v; keys that resolve to one place are one key spelled twice", limits, err, want)
	}
}

// Decimal: a binary megabyte would over-state every limit this installation
// reports by 4.8%.
func TestAMegabyteIsAMillionBytes(t *testing.T) {
	limits, err := loadUploads(t, "uploads:\n  attachment_mb: 1\n")
	if err != nil || limits.Attachment != 1_000_000 {
		t.Fatalf("1 MB resolved to %d bytes (%v), want 1000000", limits.Attachment, err)
	}
}

// A silent clamp would leave an operator who asked for 500 MB believing they
// got it. Zero is refused too: it is a ceiling no upload fits, far likelier a
// mistake than an intention.
func TestAnUnusableCeilingIsRefusedAtBootByName(t *testing.T) {
	for name, tc := range map[string]struct{ body, key string }{
		"past the range":  {"uploads:\n  attachment_mb: 500\n", "attachment_mb"},
		"negative":        {"uploads:\n  csv_import_mb: -1\n", "csv_import_mb"},
		"explicitly zero": {"uploads:\n  linkedin_import_mb: 0\n", "linkedin_import_mb"},
	} {
		if _, err := loadUploads(t, tc.body); err == nil || !strings.Contains(err.Error(), tc.key) {
			t.Errorf("%s: err %v; want a refusal naming %s", name, err, tc.key)
		}
	}
}

// `attachment_mp: 40` must not read as the operator saying nothing.
func TestATypoInTheUploadsSectionIsABootError(t *testing.T) {
	if _, err := loadUploads(t, "uploads:\n  attachment_mp: 40\n"); err == nil {
		t.Fatal("an unknown uploads key was ignored rather than refused")
	}
}

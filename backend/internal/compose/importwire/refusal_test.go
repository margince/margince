// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// oneFieldRefusal is the single field refusal err is, failing otherwise.
func oneFieldRefusal(t *testing.T, err error, field, code string) httperr.FieldError {
	t.Helper()
	refusal, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || len(refusal.Fields) != 1 || refusal.Fields[0].Field != field || refusal.Fields[0].Code != code {
		t.Fatalf("err = %v, want one %s refusal on %s", err, code, field)
	}
	return refusal.Fields[0]
}

func wantFieldParams(t *testing.T, fault httperr.FieldError, want map[string]string) {
	t.Helper()
	if !maps.Equal(fault.Params, want) {
		t.Errorf("params = %v, want %v", fault.Params, want)
	}
}

func zipHolding(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("email\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestEachFileRefusalCarriesItsCodeAndTheValuesItsSentenceNames(t *testing.T) {
	_, repeated := migration.ProfileCSV(strings.NewReader("email,email\na,b\n"), 10)
	_, ragged := migration.ProfileCSV(strings.NewReader("email,name\na@x.test\n"), 10)
	_, twoContacts := migration.OpenBundle(zipHolding(t, "export/contacts.csv", "CONTACTS.CSV"), 10<<20)
	for _, tc := range []struct {
		name string
		err  error
		code string
		want map[string]string
	}{
		{"a repeated header name", repeated, "header_unusable", map[string]string{"reason": "repeated_name", "name": "email"}},
		{"a ragged row", ragged, "unreadable", map[string]string{"reason": "field_count", "line": "2"}},
		{"a file not in UTF-8", migration.CheckUTF8([]byte("M\xfcller")), "not_utf8", nil},
		{"two files of one type", twoContacts, "bundle_refused", map[string]string{
			"reason": "two_files_one_type", "other_file": "export/contacts.csv", "file": "CONTACTS.CSV", "object": "contact",
		}},
		{
			"a header fault inside a ZIP", migration.InEntry("contacts.csv", repeated), "header_unusable",
			map[string]string{"reason": "repeated_name", "name": "email", "file": "contacts.csv"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fault := oneFieldRefusal(t, Problem(tc.err), "file", tc.code)
			wantFieldParams(t, fault, tc.want)
			if strings.Contains(fault.Message, "import source") || strings.Contains(fault.Message, "import bundle") {
				t.Errorf("message = %q, want the reader's sentence without the sentinel's words", fault.Message)
			}
		})
	}
}

func TestAVanishedUploadIsNotFound(t *testing.T) {
	err := Problem(fmt.Errorf("import source %q: %w", "ws/import/x", blobstore.ErrNotFound))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want it to carry ErrNotFound so the transport answers 404", err)
	}
	if unreadable := Problem(fmt.Errorf("%w: line 4", migration.ErrSourceUnreadable)); errors.Is(unreadable, apperrors.ErrNotFound) {
		t.Fatalf("an unreadable file was classified as missing: %v", unreadable)
	}
}

func TestTheRowCapRefusalStatesTheLimitAndTheCount(t *testing.T) {
	fault := oneFieldRefusal(t, TooManyRows(16_000, 15_000), "file", "too_many_rows")
	if fault.Max != 15_000 {
		t.Errorf("max = %d, want the cap", fault.Max)
	}
	wantFieldParams(t, fault, map[string]string{"rows": "16000"})
}

func TestAMappingWithNoKeyNamesItsKeyAndRefWhereTheyDiffer(t *testing.T) {
	wantFieldParams(t, oneFieldRefusal(t, UnmappableKey("contact", "email", "ref"), "source_key", "unmappable"),
		map[string]string{"object": "contact", "target": "email", "ref_target": "ref"})
	wantFieldParams(t, oneFieldRefusal(t, UnmappableKey("deal", "ref", "ref"), "source_key", "unmappable"),
		map[string]string{"object": "deal", "target": "ref"})
}

func TestTwoColumnsOntoOneFieldNameTheFieldAndBothColumns(t *testing.T) {
	wantFieldParams(t, oneFieldRefusal(t, DuplicateTarget("email", "alt_email", "email"), "mapping", "duplicate_target"),
		map[string]string{"target": "email", "column": "alt_email", "other_column": "email"})
}

func TestAnUnsupportedObjectNamesEveryTypeAnImportTakes(t *testing.T) {
	fault := oneFieldRefusal(t, UnsupportedObject(), "object", "unsupported")
	for _, object := range migration.ImportOrder {
		if !strings.Contains(fault.Message, object) {
			t.Errorf("message = %q, want it to name %s", fault.Message, object)
		}
	}
}

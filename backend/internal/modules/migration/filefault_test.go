// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"testing"
)

// fileFaultOf is the FileFault err carries, failing when it carries none or
// is not the sentinel kind.
func fileFaultOf(t *testing.T, err error, kind error) *FileFault {
	t.Helper()
	fault, ok := errors.AsType[*FileFault](err)
	if !ok || !errors.Is(err, kind) {
		t.Fatalf("err = %v, want a FileFault that is %v", err, kind)
	}
	return fault
}

type values = map[RefusalParam]string

func wantParams(t *testing.T, fault *FileFault, want values) {
	t.Helper()
	wanted := Params{}
	for name, value := range want {
		wanted[string(name)] = value
	}
	if !maps.Equal(fault.Params, wanted) {
		t.Errorf("params = %v, want %v", fault.Params, want)
	}
}

func TestAnUnreadableLineIsNamedWithItsCause(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		want       values
	}{
		{
			"a row with too few fields", "Email,Website\na@x.test\n",
			values{ParamReason: string(FileFieldCount), ParamLine: "2"},
		},
		{
			"a quote out of place", "Email,Website\na@x.test,x\nb@x.test,\"y\"z\n",
			values{ParamReason: string(FileQuote), ParamLine: "3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ProfileCSV(strings.NewReader(tc.file), 100)
			fault := fileFaultOf(t, err, ErrSourceUnreadable)
			wantParams(t, fault, tc.want)
			if !strings.HasPrefix(fault.Message, "Line "+tc.want[ParamLine]+" ") {
				t.Errorf("message = %q, want it to open on the line", fault.Message)
			}
		})
	}
}

func TestCountingRowsNamesTheLineItCannotRead(t *testing.T) {
	_, err := CountRecords(strings.NewReader("Email,Website\na@x.test,x\nb@x.test\n"))
	wantParams(t, fileFaultOf(t, err, ErrSourceUnreadable), values{ParamReason: string(FileFieldCount), ParamLine: "3"})
}

func TestAnUnusableHeaderSaysWhatIsWrongWithIt(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		want       values
	}{
		{"no header", "", values{ParamReason: string(FileNoHeader)}},
		{"a blank name", "Email,,Website\na,b,c\n", values{ParamReason: string(FileBlankName), ParamPosition: "2"}},
		{"a repeated name", "Email,Email\na,b\n", values{ParamReason: string(FileRepeatedName), ParamName: "Email"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ProfileCSV(strings.NewReader(tc.file), 100)
			wantParams(t, fileFaultOf(t, err, ErrHeaderInvalid), tc.want)
		})
	}
}

func TestAFileNotInUTF8NamesNoValue(t *testing.T) {
	fault := fileFaultOf(t, CheckUTF8([]byte("name\nM\xfcller\n")), ErrNotUTF8)
	if len(fault.Params) != 0 || !strings.Contains(fault.Message, "UTF-8") {
		t.Errorf("fault = %+v, want a sentence naming UTF-8 and no params", fault)
	}
}

func TestEachBundleRefusalNamesItsReasonAndFiles(t *testing.T) {
	tooMany := map[string]string{}
	for i := range MaxBundleEntries + 1 {
		tooMany["f"+strconv.Itoa(i)+".csv"] = "a\n"
	}
	for _, tc := range []struct {
		name string
		body []byte
		want values
	}{
		{"not a ZIP", []byte("PK\x03\x04 broken"), values{ParamReason: string(BundleNotAZip)}},
		{"an empty ZIP", zipOf(t, nil), values{ParamReason: string(BundleEmpty)}},
		{"too many files", zipOf(t, tooMany), values{
			ParamReason: string(BundleTooManyFiles), ParamFiles: strconv.Itoa(MaxBundleEntries + 1), ParamMaxFiles: strconv.Itoa(MaxBundleEntries),
		}},
		{
			"declared sizes past the limit", zipOf(t, map[string]string{"companies.csv": strings.Repeat("a,b\n", 300)}),
			values{ParamReason: string(BundleFilesTooLarge), ParamMaxMB: "0"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limit := int64(10 << 20)
			if tc.want[ParamReason] == string(BundleFilesTooLarge) {
				limit = 100
			}
			_, err := OpenBundle(tc.body, limit)
			wantParams(t, fileFaultOf(t, err, ErrBundleRefused), tc.want)
		})
	}
}

func TestTwoFilesOfOneTypeAreNamedAsParams(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"export/contacts.csv", "CONTACTS.CSV"} {
		if _, err := w.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := OpenBundle(buf.Bytes(), 10<<20)
	wantParams(t, fileFaultOf(t, err, ErrBundleRefused), values{
		ParamReason: string(BundleTwoFilesOneType), ParamOtherFile: "export/contacts.csv", ParamFile: "CONTACTS.CSV", ParamObject: ObjectContact,
	})
}

func TestAnEmptyZipIsAZip(t *testing.T) {
	if !IsZip(zipOf(t, nil)) {
		t.Fatal("an archive with no entries must be read as a ZIP, so it is refused as one")
	}
}

func TestInEntryNamesTheFileAndKeepsTheReason(t *testing.T) {
	_, err := ProfileCSV(strings.NewReader("Email,Email\na,b\n"), 100)
	named := InEntry("contacts.csv", err)
	fault := fileFaultOf(t, named, ErrHeaderInvalid)
	wantParams(t, fault, values{ParamReason: string(FileRepeatedName), ParamName: "Email", ParamFile: "contacts.csv"})
	if !strings.HasPrefix(fault.Message, "In “contacts.csv”, the header row names") {
		t.Errorf("message = %q, want it to name the file", fault.Message)
	}
	if original := fileFaultOf(t, err, ErrHeaderInvalid); original.Params[string(ParamFile)] != "" {
		t.Errorf("naming the entry changed the original fault: %v", original.Params)
	}
}

func TestAnUnreadableReadKeepsTheFileFaultItCarries(t *testing.T) {
	fault := fileFaultOf(t, unreadable(fmt.Errorf("reading the entry: %w", fileDamaged("contacts.csv"))), ErrBundleRefused)
	wantParams(t, fault, values{ParamReason: string(BundleFileDamaged), ParamFile: "contacts.csv"})
}

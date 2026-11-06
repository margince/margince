// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// arrivedWithMessageReaders are the statements that answer "the files this
// message carried": the paperclip count and the mail's own file list.
//
// Pinned by name because the question cannot be read off the shape.
//
// Nine statements in the tree read `attachment` for an activity. Seven must
// keep a link row: erasure reaches it, a subject access answer lists it, an
// export carries it.
//
// A census of that shape would fail on correct code. The quickest way to green
// it would be to exclude link rows from erasure.
var arrivedWithMessageReaders = map[string]bool{
	"attachmentcounts.go":  false,
	"emailpresentation.go": false,
}

// These two readers apply the fragment. Dropping it from either puts a link row
// in a paperclip count or a mail's file list.
func TestTheFilesAMessageCarriedAreAskedByTwoReaders(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		// The declaration is not an application of it.
		applies := strings.Contains(string(src), "arrivedWithMessage(\"")
		seen, listed := arrivedWithMessageReaders[name]
		switch {
		case applies && !listed:
			t.Errorf("%s applies arrivedWithMessage and is not one of the two readers that answer "+
				"what a message carried. If it is a third, add it here with what it answers", name)
		case !applies && listed && seen:
			t.Errorf("%s no longer applies arrivedWithMessage: a link row would read as a file the "+
				"message carried", name)
		}
		if applies && listed {
			arrivedWithMessageReaders[name] = true
		}
	}
	for name, found := range arrivedWithMessageReaders {
		if !found {
			t.Errorf("%s no longer applies arrivedWithMessage, or is gone: a link row would read as "+
				"a file the message carried", name)
		}
	}
}

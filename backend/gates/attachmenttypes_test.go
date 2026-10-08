// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The file picker offers the kinds of file the server keeps, no more and no fewer.
//
// An extension only the browser lists lets a reader pick a file the upload then
// refuses; one only the server lists is a kind the picker hides. Both directions
// fail here.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

const frontendAttachmentUpload = "../frontend/src/screens/attachmentupload.ts"

// tsAttachmentList is the array literal's body; tsExtension one quoted entry in it.
var (
	tsAttachmentList = regexp.MustCompile(`(?s)ACCEPTED_ATTACHMENT_EXTENSIONS\s*=\s*\[(.*?)\]`)
	tsExtension      = regexp.MustCompile(`["']([^"']*)["']`)
)

func TestTheAttachmentPickerOffersTheKindsTheServerKeeps(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendAttachmentUpload)
	if err != nil {
		t.Fatalf("reading the frontend attachment list: %v", err)
	}
	list := tsAttachmentList.FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares ACCEPTED_ATTACHMENT_EXTENSIONS as an array literal — this gate is reading a shape that is gone", frontendAttachmentUpload)
	}
	body := tsComment.ReplaceAll(list[1], []byte(" "))
	var inTS []string
	for _, m := range tsExtension.FindAllSubmatch(body, -1) {
		inTS = append(inTS, string(m[1]))
	}
	if len(inTS) == 0 {
		t.Fatal("no extensions parsed out of the frontend list — a gate that reads nothing agrees with everything")
	}

	inGo := activities.AcceptedAttachmentExtensions()
	for _, ext := range inGo {
		if !slices.Contains(inTS, ext) {
			t.Errorf("%s is accepted by the server and missing from the picker, so a reader cannot choose a file the upload would keep", ext)
		}
	}
	for _, ext := range inTS {
		if !slices.Contains(inGo, ext) {
			t.Errorf("%s is offered by the picker and refused by the server, so a reader can choose a file the upload will turn away", ext)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The file picker offers the kinds of file the server keeps, no more and no fewer,
// by ending and by declared media type.
//
// An entry only the browser lists lets a reader pick a file the upload then
// refuses. One only the server lists is a kind the picker hides. Both directions
// fail here.

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendAttachmentUpload = "../frontend/src/lib/attachmentupload.ts"

// tsAttachmentEntry is one quoted entry of an attachment list.
var tsAttachmentEntry = regexp.MustCompile(`["']([^"']*)["']`)

func TestTheAttachmentPickerOffersTheKindsTheServerKeeps(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendAttachmentUpload)
	if err != nil {
		t.Fatalf("reading the frontend attachment list: %v", err)
	}
	for name, inGo := range map[string][]string{
		"ACCEPTED_ATTACHMENT_EXTENSIONS":  activities.AcceptedAttachmentExtensions(),
		"ACCEPTED_ATTACHMENT_MEDIA_TYPES": activities.AcceptedAttachmentMediaTypes(),
	} {
		inTS := tsArrayEntries(t, source, name)
		for _, entry := range inGo {
			if !slices.Contains(inTS, entry) {
				t.Errorf("%s: %s is accepted by the server and missing from the picker, so a reader cannot choose a file the upload would keep", name, entry)
			}
		}
		for _, entry := range inTS {
			if !slices.Contains(inGo, entry) {
				t.Errorf("%s: %s is offered by the picker and refused by the server, so a reader can choose a file the upload will turn away", name, entry)
			}
		}
	}
}

// tsArrayEntries reads the quoted entries of the array literal bound to name.
func tsArrayEntries(t *testing.T, source []byte, name string) []string {
	t.Helper()
	list := regexp.MustCompile(`(?s)` + name + `\s*=\s*\[(.*?)\]`).FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares %s as an array literal — this gate is reading a shape that is gone", frontendAttachmentUpload, name)
	}
	var entries []string
	for _, m := range tsAttachmentEntry.FindAllSubmatch(tsComment.ReplaceAll(list[1], []byte(" ")), -1) {
		entries = append(entries, string(m[1]))
	}
	if len(entries) == 0 {
		t.Fatalf("no entries parsed out of %s — a gate that reads nothing agrees with everything", name)
	}
	return entries
}

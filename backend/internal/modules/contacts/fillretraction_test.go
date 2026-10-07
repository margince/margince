// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A fill names fields that were empty and are profile fields, read from a
// source the undo can find again; anything else is an ordinary change.
func TestAFillIsReadFromItsImagesAndItsSource(t *testing.T) {
	at := time.Now()
	sig := json.RawMessage(`{"source":"capture_enrich","source_ref":"activity:x","confirmed":["title"]}`)
	fill, ok := FillOf(json.RawMessage(`{"title":null,"phone":null}`),
		json.RawMessage(`{"title":"filled","phone":"filled"}`), sig, at)
	if !ok || !slices.Equal(fill.Fields, []string{"phone"}) || fill.Legacy || fill.MirroredTitle != "" {
		t.Errorf("signature fill = %+v ok=%v, want phone only (title confirmed), not legacy, no mirrored title", fill, ok)
	}
	site := json.RawMessage(`{"source":"site_read","source_ref":"site_read:https://acme.test"}`)
	fill, ok = FillOf(json.RawMessage(`{"title":null}`), json.RawMessage(`{"title":"CTO"}`), site, at)
	if !ok || fill.MirroredTitle != "CTO" || fill.Legacy {
		t.Errorf("site fill = %+v ok=%v, want the mirrored title CTO", fill, ok)
	}
	legacy := json.RawMessage(`{"source":"capture_enrich","source_ref":"activity:x"}`)
	if fill, _ = FillOf(json.RawMessage(`{"title":null}`), json.RawMessage(`{"title":"filled"}`), legacy, at); !fill.Legacy {
		t.Error("a signature without a confirmed list is not read as legacy")
	}
	for name, images := range map[string][2]string{
		"a replacement":     {`{"title":"CEO"}`, `{"title":"CTO"}`},
		"not a profile key": {`{"full_name":null}`, `{"full_name":"Ann"}`},
		"unbalanced":        {`{}`, `{"title":"CTO"}`},
	} {
		if _, ok := FillOf(json.RawMessage(images[0]), json.RawMessage(images[1]), site, at); ok {
			t.Errorf("%s is read as a fill", name)
		}
	}
	if _, ok := FillOf(json.RawMessage(`{"title":null}`), json.RawMessage(`{"title":"x"}`),
		json.RawMessage(`{"source":"manual","source_ref":"x"}`), at); ok {
		t.Error("a source the undo cannot find again is read as a fill")
	}
}

// A refusal names the fields it is about, and no entry orders by nothing.
func TestARefusalNamesItsFieldsAndNoEntryOrdersByTimeAlone(t *testing.T) {
	msg := (&FillRetractionRefusal{Moved: []string{"title"}, Replaced: []string{"phone"}}).Error()
	if !strings.Contains(msg, "title") || !strings.Contains(msg, "phone") {
		t.Errorf("refusal %q does not name its fields", msg)
	}
	if entryOrNone(ids.Nil) != nil {
		t.Error("the zero entry orders audit rows by id")
	}
	entry := ids.NewV7()
	if got := entryOrNone(entry); got == nil || *got != entry {
		t.Errorf("entry = %v, want %v", got, entry)
	}
}

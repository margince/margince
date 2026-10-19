// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every slot pkg/extension accepts is mounted on a core screen, read from the
// screens themselves. A slot the list names and no screen mounts is an entry
// that passes every check and never shows.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const entriesElement = "<ExtensionEntries"

// slotAttr is a literal slot prop in an element's own attributes.
var slotAttr = regexp.MustCompile(`(?:^|\s)slot="([^"]*)"`)

// mountedEntrySlots returns the slot of every `<ExtensionEntries` element in
// product code under root. Tests and stories are left out, since they mount
// the component on fixtures rather than on a screen. An element whose slot is
// not a literal is an error: a slot this gate cannot read could be any slot.
func mountedEntrySlots(root string) ([]string, error) {
	var slots []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".tsx") ||
			strings.Contains(path, ".test.") || strings.Contains(path, ".stories.") {
			return nil
		}
		source, err := os.ReadFile(path) // #nosec G304 -- a path the walk of the frontend tree produced
		if err != nil {
			return err
		}
		found, err := elementSlots(string(source))
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, slot := range found {
			if !slices.Contains(slots, slot) {
				slots = append(slots, slot)
			}
		}
		return nil
	})
	return slots, err
}

// elementSlots reads each element's attributes up to the `>` that closes its
// tag at brace depth zero. An arrow or a comparison inside a prop expression
// then cannot end the element early. Only top-level attributes count.
func elementSlots(source string) ([]string, error) {
	var slots []string
	for rest := source; ; {
		start := strings.Index(rest, entriesElement)
		if start < 0 {
			return slots, nil
		}
		rest = rest[start+len(entriesElement):]
		if rest == "" || !strings.ContainsRune(" \t\n/>", rune(rest[0])) {
			continue
		}
		var top strings.Builder
		depth, end := 0, -1
		for i, r := range rest {
			switch {
			case r == '{':
				depth++
			case r == '}':
				depth--
			case r == '>' && depth == 0:
				end = i
			}
			if end >= 0 {
				break
			}
			if depth == 0 && r != '}' {
				top.WriteRune(r)
			}
		}
		if end < 0 {
			return nil, fmt.Errorf("an %s element never closes", entriesElement)
		}
		m := slotAttr.FindStringSubmatch(top.String())
		if m == nil {
			return nil, fmt.Errorf("an %s element has no literal slot prop, so no gate can tell which slot it mounts", entriesElement)
		}
		slots = append(slots, m[1])
		rest = rest[end:]
	}
}

func TestElementSlotsReadsEveryShapeOfMount(t *testing.T) {
	t.Parallel()
	source := `
	<ExtensionEntries slot="record_header.deal" record={{ type: "deal", id: deal.id }} />
	<ExtensionEntries
	  inRow
	  record={entityType && entityId ? { type: entityType, id: entityId } : undefined}
	  onPick={() => a > b}
	  slot="log_activity"
	/>
	<ExtensionEntriesHelp slot="not_a_mount" />`
	got, err := elementSlots(source)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"record_header.deal", "log_activity"}; !slices.Equal(got, want) {
		t.Fatalf("slots = %v, want %v", got, want)
	}
	if _, err := elementSlots(`<ExtensionEntries slot={chosen} />`); err == nil {
		t.Error("a slot passed as an expression was read as no slot at all")
	}
}

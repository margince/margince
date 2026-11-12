// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package toolroles reads and writes what the MCP role sweep publishes: one
// file per verb in each of two folders. Two changes to different verbs then
// never touch one file. The agents module embeds the checks folder; the page
// and its gates read the rows folder from docs.
package toolroles

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// HeadFile is the one file in a folder that belongs to no verb. It holds the
// note and, for the rows, the reasons and roles every row is read against.
const HeadFile = "_head.json"

// Checks is one verb's file in the checks folder.
type Checks struct {
	Verb   string   `json:"verb"`
	Checks []string `json:"checks"`
	// Takes is the kinds of record the verb's call is given by the seeds.
	Takes []string `json:"takes,omitempty"`
}

// Row is one verb's file in the rows folder.
type Row struct {
	Tool    string   `json:"tool"`
	Action  string   `json:"action,omitempty"`
	Verb    string   `json:"verb"`
	Allowed []string `json:"allowed_for"`
	Refused []string `json:"refused_for"`
	// ColleagueRecord is what each scoped role was answered on a record a
	// colleague owns, for a call that takes one.
	ColleagueRecord map[string]string `json:"colleague_record,omitempty"`
}

// Head is the rows folder's head file; the checks folder's carries Note only.
type Head struct {
	Note    string            `json:"note"`
	Reasons map[string]string `json:"reasons,omitempty"`
	Roles   []string          `json:"roles,omitempty"`
}

type verbFile interface{ Checks | Row }

type anyFile interface{ Head | Checks | Row }

func verbOf[T verbFile](v T) string {
	switch f := any(v).(type) {
	case Checks:
		return f.Verb
	case Row:
		return f.Verb
	}
	return ""
}

// Read reads a folder's head and its verb files, keyed by verb. A file whose
// name is not its verb is an error, so a rename cannot leave a stale copy.
func Read[T verbFile](fsys fs.FS) (Head, map[string]T, error) {
	head, err := decode[Head](fsys, HeadFile)
	if err != nil {
		return head, nil, err
	}
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return head, nil, err
	}
	out := map[string]T{}
	for _, name := range names {
		if name == HeadFile {
			continue
		}
		v, err := decode[T](fsys, name)
		if err != nil {
			return head, nil, err
		}
		if verb := verbOf(v); name != verb+".json" {
			return head, nil, fmt.Errorf("toolroles: %s holds the verb %q; a verb's file is named for it", name, verb)
		}
		out[verbOf(v)] = v
	}
	return head, out, nil
}

func decode[T anyFile](fsys fs.FS, name string) (T, error) {
	var into T
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return into, err
	}
	if err := json.Unmarshal(raw, &into); err != nil {
		return into, fmt.Errorf("toolroles: %s does not read: %w", name, err)
	}
	return into, nil
}

// Render is the folder as the sweep writes it: the head and one file per verb.
func Render[T verbFile](head Head, verbs map[string]T) (map[string][]byte, error) {
	out := map[string][]byte{}
	raw, err := encode(head)
	if err != nil {
		return nil, err
	}
	out[HeadFile] = raw
	for verb, v := range verbs {
		if out[verb+".json"], err = encode(v); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func encode[T anyFile](v T) ([]byte, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	return append(raw, '\n'), err
}

// Drift is how a folder on disk differs from what the sweep rendered, by file.
type Drift struct{ Missing, Extra, Changed []string }

// Empty reports whether the folder matches.
func (d Drift) Empty() bool { return len(d.Missing)+len(d.Extra)+len(d.Changed) == 0 }

func (d Drift) String() string {
	var parts []string
	for _, part := range []struct {
		name  string
		files []string
	}{{"missing", d.Missing}, {"extra", d.Extra}, {"changed", d.Changed}} {
		if len(part.files) > 0 {
			parts = append(parts, part.name+": "+strings.Join(part.files, ", "))
		}
	}
	return strings.Join(parts, "; ")
}

// Compare is the drift between dir and want, file by file.
func Compare(dir string, want map[string][]byte) (Drift, error) {
	var drift Drift
	have, err := onDisk(dir)
	if err != nil {
		return drift, err
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		got, ok := have[name]
		switch {
		case !ok:
			drift.Missing = append(drift.Missing, name)
		case !bytes.Equal(got, want[name]):
			drift.Changed = append(drift.Changed, name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(have)) {
		if _, ok := want[name]; !ok {
			drift.Extra = append(drift.Extra, name)
		}
	}
	return drift, nil
}

// Write makes dir hold want and no other JSON file. A verb no longer served
// then leaves no file behind.
func Write(dir string, want map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	have, err := onDisk(dir)
	if err != nil {
		return err
	}
	var errs []error
	for name := range have {
		if _, ok := want[name]; !ok {
			errs = append(errs, os.Remove(filepath.Join(dir, name)))
		}
	}
	for name, raw := range want {
		errs = append(errs, os.WriteFile(filepath.Join(dir, name), raw, 0o600))
	}
	return errors.Join(errs...)
}

func onDisk(dir string) (map[string][]byte, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, path := range names {
		raw, err := os.ReadFile(path) // #nosec G304 -- the sweep's own output folder
		if err != nil {
			return nil, err
		}
		out[filepath.Base(path)] = raw
	}
	return out, nil
}

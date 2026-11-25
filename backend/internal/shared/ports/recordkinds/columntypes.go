// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordkinds

import (
	"fmt"
)

// CheckColumns refuses a kind whose table lacks a column the kind reads. It
// also refuses a title or timeline column whose type History and search cannot
// read. The title is searched as text, the time orders the History, and a
// subject column is compared to an id.
func CheckColumns(k Declared, table string, columns map[string]string) error {
	required := []string{"id", "archived_at"}
	for _, f := range k.Fields {
		required = append(required, f.Name)
	}
	for _, c := range required {
		if _, ok := columns[c]; !ok {
			return fmt.Errorf("record kind %s: table %s has no column %q", k.Name, table, c)
		}
	}
	title := k.NaturalKey[0]
	if tl := k.Timeline; tl != nil {
		title = tl.LabelField
		if err := wantColumnType(k, table, columns, tl.AtColumn, "timeline time", "timestamp with time zone"); err != nil {
			return err
		}
		for _, s := range tl.Subjects {
			if err := wantColumnType(k, table, columns, s.Column, "timeline "+s.Entity, "uuid"); err != nil {
				return err
			}
		}
	}
	if typ, ok := columns[title]; ok && typ != "text" && typ != "character varying" && typ != "citext" {
		return fmt.Errorf("record kind %s: title column %q of %s is %s, but search reads it as text", k.Name, title, table, typ)
	}
	return nil
}

func wantColumnType(k Declared, table string, columns map[string]string, col, role, want string) error {
	typ, ok := columns[col]
	if !ok {
		return fmt.Errorf("record kind %s: %s column %q does not exist in %s", k.Name, role, col, table)
	}
	if typ != want {
		return fmt.Errorf("record kind %s: %s column %q of %s is %s, not %s", k.Name, role, col, table, typ, want)
	}
	return nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// settingKeyFor is the setting a field of OperationSettings or OperationPatch
// carries, by name: AgentRunnerIntervalSeconds is
// installation.agent_runner_interval_seconds.
func settingKeyFor(field string) string {
	var key strings.Builder
	key.WriteString("installation.")
	for i, r := range field {
		if unicode.IsUpper(r) && i > 0 {
			key.WriteByte('_')
		}
		key.WriteRune(unicode.ToLower(r))
	}
	return key.String()
}

// filledOperationPatch sets field i of an OperationPatch to i+1, by reflection
// so no field can be missed.
func filledOperationPatch(t *testing.T) OperationPatch {
	t.Helper()
	var patch OperationPatch
	value := reflect.ValueOf(&patch).Elem()
	for i := range value.NumField() {
		if value.Field(i).Type() != reflect.TypeFor[*int]() {
			t.Fatalf("OperationPatch.%s is %s, want *int", value.Type().Field(i).Name, value.Field(i).Type())
		}
		number := i + 1
		value.Field(i).Set(reflect.ValueOf(&number))
	}
	return patch
}

func TestEveryOperationValueIsReadFromItsOwnSetting(t *testing.T) {
	var read OperationSettings
	fieldNames := map[uintptr]string{}
	value := reflect.ValueOf(&read).Elem()
	for i := range value.NumField() {
		fieldNames[value.Field(i).Addr().Pointer()] = value.Type().Field(i).Name
	}
	fields := read.fields()
	if len(fields) != len(fieldNames) {
		t.Fatalf("fields() pairs %d values, OperationSettings has %d; an unpaired one always reads zero", len(fields), len(fieldNames))
	}
	seen := map[uintptr]bool{}
	for _, field := range fields {
		pointer := reflect.ValueOf(field.into).Pointer()
		if seen[pointer] {
			t.Errorf("OperationSettings.%s is read twice, so another value is never read", fieldNames[pointer])
		}
		seen[pointer] = true
		name := fieldNames[pointer]
		if want := settingKeyFor(name); field.entry.Key() != want {
			t.Errorf("OperationSettings.%s reads %s, want %s", name, field.entry.Key(), want)
		}
	}
}

func TestEveryOperationPatchFieldWritesItsOwnSetting(t *testing.T) {
	writes, err := filledOperationPatch(t).writes()
	if err != nil {
		t.Fatalf("encoding a full patch: %v", err)
	}
	written := map[string]string{}
	for _, write := range writes {
		written[write.entry.Key()] = string(write.raw)
	}
	patchType := reflect.TypeFor[OperationPatch]()
	if len(written) != patchType.NumField() {
		t.Fatalf("a full patch writes %d distinct settings for %d fields; a field it does not write silently stops saving", len(written), patchType.NumField())
	}
	for i := range patchType.NumField() {
		key := settingKeyFor(patchType.Field(i).Name)
		if got, want := written[key], strconv.Itoa(i+1); got != want {
			t.Errorf("OperationPatch.%s wrote %q to %s, want %q", patchType.Field(i).Name, got, key, want)
		}
	}
}

func TestAnEmptyOperationPatchWritesNothing(t *testing.T) {
	writes, err := OperationPatch{}.writes()
	if err != nil {
		t.Fatalf("encoding an empty patch: %v", err)
	}
	for _, write := range writes {
		if write.raw != nil {
			t.Errorf("%s encoded %s from an absent field", write.entry.Key(), write.raw)
		}
	}
}

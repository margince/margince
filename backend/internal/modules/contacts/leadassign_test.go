// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Every field of UpdateLeadInput but the owner, the version and the trail writes
// the lead. Setting one makes the patch more than an owner change.
func TestEveryLeadFieldButTheOwnerMakesAPatchMoreThanAnOwnerChange(t *testing.T) {
	owner := ids.From[ids.UserKind](ids.NewV7())
	base := UpdateLeadInput{OwnerID: &owner, IfVersion: new(int64(3))}
	if !ownershipOnlyLeadUpdate(base) {
		t.Fatal("an owner and a version alone read as more than an owner change")
	}
	notWritten := map[string]bool{"OwnerID": true, "IfVersion": true, "Trail": true}
	fields := reflect.TypeFor[UpdateLeadInput]()
	for i := range fields.NumField() {
		field := fields.Field(i)
		if notWritten[field.Name] {
			continue
		}
		in := base
		value := reflect.ValueOf(&in).Elem().Field(i)
		switch value.Kind() {
		case reflect.Pointer:
			value.Set(reflect.New(field.Type.Elem()))
		case reflect.Slice:
			value.Set(reflect.MakeSlice(field.Type, 1, 1))
		case reflect.Map:
			value.Set(reflect.MakeMap(field.Type))
			value.SetMapIndex(reflect.Zero(field.Type.Key()), reflect.Zero(field.Type.Elem()))
		case reflect.String:
			value.SetString("set")
		case reflect.Bool:
			value.SetBool(true)
		default:
			t.Fatalf("UpdateLeadInput.%s is a %s, which this test cannot set; teach it the kind", field.Name, value.Kind())
		}
		if ownershipOnlyLeadUpdate(in) {
			t.Errorf("a patch setting %s reads as an owner change alone", field.Name)
		}
	}
}

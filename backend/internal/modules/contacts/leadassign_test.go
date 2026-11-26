// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
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

// The bulk destination tells an explicit null, the unassigned queue, from an
// owner the body never named, which is refused rather than read as nobody.
func TestTheBulkOwnerTellsNullFromAbsent(t *testing.T) {
	seat := ids.NewV7()
	for _, tc := range []struct {
		name, body string
		want       *ids.UserID
		refused    bool
	}{
		{"a seat is that seat", `{"owner_id":"` + seat.String() + `","leads":[]}`, new(ids.From[ids.UserKind](seat)), false},
		{"null is the queue", `{"owner_id":null,"leads":[]}`, nil, false},
		{"absent is refused", `{"leads":[]}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/leads/assign-bulk", strings.NewReader(tc.body))
			var req crmcontracts.AssignLeadsRequest
			if !httperr.Decode(httptest.NewRecorder(), r, &req) {
				t.Fatalf("the body %s did not decode", tc.body)
			}
			got, err := bulkOwner(r, req.OwnerId)
			if (err != nil) != tc.refused || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("bulkOwner = %v, %v; want %v, refused %v", got, err, tc.want, tc.refused)
			}
		})
	}
}

// A bulk run with no owner clears each lead's owner through the same update a
// PATCH with `owner_id: null` makes.
func TestAHandOverToNobodyClearsTheOwner(t *testing.T) {
	version := int64(4)
	got := handOver(nil, &version)
	want := UpdateLeadInput{Clear: []string{"owner_id"}, IfVersion: &version}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("handOver(nil) = %+v, want %+v", got, want)
	}
}

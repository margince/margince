// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestMemberWorkCountReachesEveryContractCount(t *testing.T) {
	var counts crmcontracts.HandoverCounts
	fields := reflect.ValueOf(&counts).Elem()
	if fields.NumField() == 0 {
		t.Fatal("the handover contract has no counts")
	}
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		if field.Kind() != reflect.Int {
			t.Fatalf("handover field %s is not a count", fields.Type().Field(i).Name)
		}
		field.SetInt(1)
		if got := MemberWorkCount(counts); got != 1 {
			t.Errorf("%s contributed %d", fields.Type().Field(i).Name, got)
		}
		field.SetInt(0)
	}
}

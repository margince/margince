// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// searchTypesNotOffered are the GET /v1/search types search_records does not
// take, each with the door a passport asks through instead.
var searchTypesNotOffered = gatekit.Waive(map[string]string{
	"tag": "read_record cannot read a tag; read_tags is the door to the tag vocabulary",
})

// search_records takes every type GET /v1/search takes, but the ones it names
// a reason for. A type the route gains is then offered, or its absence argued.
func TestSearchRecordsTakesEverySearchTypeButTheOnesItArgues(t *testing.T) {
	var routeTypes []string
	for _, enum := range queryEnumTable["GET /v1/search"] {
		if enum.Name == "types" {
			routeTypes = enum.Values
		}
	}
	if len(routeTypes) == 0 {
		t.Fatal("GET /v1/search declares no types enum, so nothing is compared")
	}
	spec, ok := NewRegistry(nil, SendPath{}).Spec("search_records")
	if !ok {
		t.Fatal("search_records is not registered")
	}
	var schema struct {
		Properties struct {
			RecordTypes struct {
				Items struct {
					Enum []string `json:"enum"`
				} `json:"items"`
			} `json:"record_types"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		t.Fatalf("search_records publishes an input schema that is not JSON: %v", err)
	}
	offered := schema.Properties.RecordTypes.Items.Enum
	for _, routeType := range routeTypes {
		if !slices.Contains(offered, routeType) && !searchTypesNotOffered.Waived(t, routeType) {
			t.Errorf("GET /v1/search takes %q, and search_records neither takes it nor argues why not", routeType)
		}
	}
	searchTypesNotOffered.AssertAllMatched(t)
	for _, recordType := range offered {
		if !slices.Contains(routeTypes, recordType) {
			t.Errorf("search_records takes %q, which GET /v1/search does not search", recordType)
		}
	}
}

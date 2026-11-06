// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/platform/workarea"
)

// Every tool the full registry serves an agent by name sits in an area. A
// limited passport is then never refused one for want of a placement. A router
// is admitted on its actions' verbs, and a tool about the caller or the
// protocol sits in no area of work.
func TestEveryCoreToolIsPlacedInAnArea(t *testing.T) {
	t.Parallel()
	specs := NewRegistry(nil, SendPath{}).Specs()
	if len(specs) < 300 {
		t.Fatalf("the registry serves %d tools; the census lost them", len(specs))
	}
	for _, spec := range specs {
		if len(spec.Actions) > 0 || spec.HumanOnly || spec.SelfDescribing || spec.ProtocolOnly {
			continue
		}
		if len(spec.Areas) == 0 {
			t.Errorf("%s is registered in no area, so every passport limited to chosen areas is refused it. "+
				"Give it an x-mcp-tool operation in api/crm.yaml.", spec.Name)
		}
	}
}

// A verb in several areas places each call by its record type, so every record
// type its schema offers must have a place.
func TestEveryRecordTypeAVerbInSeveralAreasTakesIsPlaced(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if len(spec.Areas) < 2 {
			continue
		}
		var schema struct {
			Properties struct {
				RecordType struct {
					Enum []string `json:"enum"`
				} `json:"record_type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", spec.Name, err)
		}
		for _, recordType := range schema.Properties.RecordType.Enum {
			checked++
			if _, placed := workarea.OfRecordCall(spec.Name, recordType); !placed {
				t.Errorf("%s takes record_type %q, which the work-area table does not place", spec.Name, recordType)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no verb in several areas offers a record type; the census lost them")
	}
}

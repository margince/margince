// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingprofile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAProfileCitingLinesTheTranscriptHasIsRead(t *testing.T) {
	raw := json.RawMessage(`[{"field":"buying_center","value":"The plant manager decides, finance signs off.","source_lines":[3,4],"confidence":0.8},` +
		`{"field":"sales_motion","value":"They sell through regional distributors.","source_lines":[7],"confidence":0.7}]`)
	entries, fault := Read(raw, 8)
	if fault != "" || len(entries) != 2 || entries[0].Field != "buying_center" {
		t.Fatalf("Read = %+v, %q; want both fields", entries, fault)
	}
}

func TestAnEmptyProfileIsAnAnswer(t *testing.T) {
	entries, fault := Read(json.RawMessage(`[]`), 8)
	if fault != "" || len(entries) != 0 {
		t.Fatalf("Read([]) = %+v, %q; want no fields and no fault", entries, fault)
	}
}

func TestAProfileTheTranscriptCannotSupportIsRefused(t *testing.T) {
	long := strings.Repeat("x", MaxValue+1)
	for _, tc := range []struct{ name, raw, want string }{
		{"a field a meeting may not fill", `[{"field":"legal_name","value":"Acme GmbH","source_lines":[1],"confidence":0.9}]`, "may not fill"},
		{"the same field twice", `[{"field":"icp","value":"Banks","source_lines":[1],"confidence":0.9},{"field":"icp","value":"Insurers","source_lines":[2],"confidence":0.9}]`, "twice"},
		{"an empty value", `[{"field":"icp","value":" ","source_lines":[1],"confidence":0.9}]`, "empty"},
		{"a value too long", `[{"field":"icp","value":"` + long + `","source_lines":[1],"confidence":0.9}]`, "longer"},
		{"a line past the end", `[{"field":"icp","value":"Banks","source_lines":[9],"confidence":0.9}]`, "lines 1 to 8"},
		{"no line", `[{"field":"icp","value":"Banks","source_lines":[],"confidence":0.9}]`, "cites no line"},
		{"confidence above one", `[{"field":"icp","value":"Banks","source_lines":[1],"confidence":1.5}]`, "outside"},
		{"no profile at all", ``, "no company profile"},
		{"the wrong shape", `{"icp":"Banks"}`, "JSON shape"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries, fault := Read(json.RawMessage(tc.raw), 8)
			if entries != nil || !strings.Contains(fault, tc.want) {
				t.Errorf("Read = %+v, %q; want no fields and a fault containing %q", entries, fault, tc.want)
			}
		})
	}
}

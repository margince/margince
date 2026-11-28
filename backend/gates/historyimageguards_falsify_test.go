// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H3

package gates

import (
	"slices"
	"testing"
)

func TestHistoryMapKeyGuardsConstrainOnlyTheirOwnWrites(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, write string
		raw         bool
	}{
		{"excluded key", `if key!="raw"{out[key]=value}`, false},
		{"reversed comparison", `if "raw"!=key{out[key]=value}`, false},
		{"selected key", `if key=="name"{out[key]=value}`, false},
		{"nested guard", `if value!=nil{if key!="raw"{out[key]=value}}`, false},
		{"write outside guard", `if key!="raw"{};out[key]=value`, true},
		{"guard can be bypassed", `if key!="raw"||value!=nil{out[key]=value}`, true},
		{"another variable", `other:="name";if other!="raw"{out[key]=value}`, true},
		{"unconditional overwrite", `if key!="raw"{out[key]=value};out["raw"]=value`, true},
		{"key changes inside guard", `if key!="raw"{key="raw";out[key]=value}`, true},
		{"aliased key changes", `alias:=&key;if key!="raw"{*alias="raw";out[key]=value}`, true},
		{"delayed closure", `var delayed func();if key!="raw"{delayed=func(){out[key]=value}};key="raw";if delayed!=nil{delayed()}`, true},
		{"closure changes key", `mutate:=func(){key="raw"};if key!="raw"{mutate();out[key]=value}`, true},
		{"call changes key", `mutate:=func(v *string){*v="raw"};if key!="raw"{mutate(&key);out[key]=value}`, true},
		{"range reassigns key", `if key!="raw"{for key,value=range map[string]any{"raw":3}{};out[key]=value}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `func image()map[string]any{in:=map[string]any{"name":1,"raw":2};out:=make(map[string]any);for key,value:=range in{` + tc.write + `};return out};func w(){_=k.AuditEvent(ctx,tx,"update","contact",1,image())}`
			keys := keysOfPlantedWriter(t, body)
			if !tc.raw && !slices.Contains(keys, "name") {
				t.Fatalf("lost the ordinary field: %v", keys)
			}
			if slices.Contains(keys, "raw") != tc.raw {
				t.Fatalf("source field present=%v, want %v: %v", slices.Contains(keys, "raw"), tc.raw, keys)
			}
		})
	}
}

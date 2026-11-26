// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

// orderRead is a read addressing one record by an id in its path.
func orderRead(route, schema string) Verb {
	v := wellFormed()
	v.Method = http.MethodGet
	v.RequestedScope = ScopeRead
	v.RbacAction = RbacRead
	v.Route = route
	v.InputSchema = json.RawMessage(schema)
	return v
}

const orderIDSchema = `{"type":"object","properties":{"order_id":{"type":"string"}},"required":["order_id"]}`

func TestARouteMayCarryAnIDAfterItsFirstSegment(t *testing.T) {
	v := orderRead("/ext/crm-demo/orders/{order_id}", orderIDSchema)
	if err := v.Validate(); err != nil {
		t.Fatalf("a route with an id after a fixed segment must validate: %v", err)
	}
	if got := v.PathArguments(); !slices.Equal(got, []string{"order_id"}) {
		t.Errorf("path arguments = %v, want [order_id]", got)
	}
}

func TestARoutePathArgumentTheServerCannotServeIsRefused(t *testing.T) {
	for name, v := range map[string]Verb{
		"an id right after the unit":  orderRead("/ext/crm-demo/{order_id}", orderIDSchema),
		"an empty name":               orderRead("/ext/crm-demo/orders/{}", orderIDSchema),
		"an upper-case name":          orderRead("/ext/crm-demo/orders/{Order}", orderIDSchema),
		"a name the schema lacks":     orderRead("/ext/crm-demo/orders/{id}", orderIDSchema),
		"a name used twice":           orderRead("/ext/crm-demo/orders/{order_id}/{order_id}", orderIDSchema),
		"an optional argument":        orderRead("/ext/crm-demo/orders/{order_id}", `{"type":"object","properties":{"order_id":{"type":"string"}}}`),
		"a structured body argument":  bodyOrder(`{"type":"object","properties":{"order_id":{"type":"object"}},"required":["order_id"]}`),
		"a body argument it does not": bodyOrder(`{"type":"object"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := v.Validate(); err == nil {
				t.Fatal("validated; want a refusal")
			}
		})
	}
	if err := bodyOrder(orderIDSchema).Validate(); err != nil {
		t.Fatalf("a body operation with a declared path argument must validate: %v", err)
	}
}

func bodyOrder(schema string) Verb {
	v := wellFormed()
	v.Route = "/ext/crm-demo/orders/{order_id}"
	v.InputSchema = json.RawMessage(schema)
	return v
}

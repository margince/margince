// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector_test

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// The core's file types ARE the published ones, not copies of them. A fork would
// let a unit's file and a core connector's file drift apart field by field while
// every call site still compiled, which is the failure the alias exists to make
// impossible.
func TestCoreFileTypesAreThePublishedTypes(t *testing.T) {
	for _, c := range []struct {
		name      string
		core, pub reflect.Type
	}{
		{"Part", reflect.TypeFor[connector.Part](), reflect.TypeFor[extension.InboundFile]()},
		{"PartDrop", reflect.TypeFor[connector.PartDrop](), reflect.TypeFor[extension.FileDrop]()},
		{"OutboundFile", reflect.TypeFor[connector.OutboundFile](), reflect.TypeFor[extension.OutboundFile]()},
	} {
		if c.core != c.pub {
			t.Errorf("connector.%s is %v, not the published %v: an alias was replaced by a copy",
				c.name, c.core, c.pub)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The export reads activity ids off the rows readableValue rendered. An id in
// any other shape is a fault in that rendering, refused before any query runs.
func TestTheExtensionExportRefusesAnActivityIDItCannotRead(t *testing.T) {
	for name, id := range map[string]any{
		"not text":   42,
		"not a uuid": "not-a-uuid",
	} {
		t.Run(name, func(t *testing.T) {
			rows, err := exportExtensionRows(t.Context(), nil, extprivacy.Registry{}, ids.ContactID{}, nil,
				[]map[string]any{{"id": id}})
			if err == nil {
				t.Errorf("exported %v from an activity id %#v; want a refusal", rows, id)
			}
		})
	}
}

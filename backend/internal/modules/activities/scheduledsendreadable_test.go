// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The records query reads the keys the frozen links are written under. A
// renamed field would leave every account send filed under nothing.
func TestTheRecordsQueryReadsTheKeysTheFrozenLinksCarry(t *testing.T) {
	raw, err := marshalOriginLinks(SendOrigin{links: []ActivityLinkInput{{EntityType: "deal", EntityID: ids.NewV7()}}})
	if err != nil {
		t.Fatal(err)
	}
	var frozen []map[string]any
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	query := ScheduledSendRecordsSQL("placed", "deal")
	for key := range frozen[0] {
		if !strings.Contains(query, "'"+key+"'") {
			t.Errorf("the frozen link carries %q, which the records query does not read", key)
		}
	}
}

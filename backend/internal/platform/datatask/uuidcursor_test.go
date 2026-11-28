// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAUUIDCursorStartsAtTheFirstRowAndRefusesAnythingButAnID(t *testing.T) {
	t.Parallel()
	if after, err := UUIDCursor(nil); err != nil || after != ids.Nil {
		t.Fatalf("an empty cursor reads %v (err %v), want the first row", after, err)
	}
	var malformed *storekit.MalformedCursorError
	if _, err := UUIDCursor(json.RawMessage(`"invalid"`)); !errors.As(err, &malformed) {
		t.Fatalf("a cursor that names no id answered %v, want a malformed cursor", err)
	}
	id := ids.NewV7()
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if after, err := UUIDCursor(raw); err != nil || after != id {
		t.Fatalf("the cursor for %v reads %v (err %v)", id, after, err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"errors"
	"testing"
)

// A list needs a name a reader can see. The name is refused before anything
// reads the database, so a store with no pool is enough.
func TestAListNamedOnlyByInvisibleCharactersIsRefused(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", " ", "\u200b", "\u200b\u2060"} {
		err := (&Store{}).checkNewList(t.Context(), &CreateListInput{Name: name, EntityType: "contact"})
		var refusal *BadInputError
		if !errors.As(err, &refusal) || refusal.Field != nameField {
			t.Errorf("name %q answered %v, want the name refused", name, err)
		}
	}
}

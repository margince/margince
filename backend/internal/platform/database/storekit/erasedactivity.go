// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// ErasedActivitySubject is the tombstone the erase executor leaves in an
// activity's subject line, and in the subject of the delivery that sent it.
// It lives here so a selector in another module can recognise the tombstone
// without importing the module that writes it.
const ErasedActivitySubject = "Erased"

// ActivityErasedSQL is true when the activity aliased activity was already
// erased. The erase leaves the tombstone subject, no body and no original.
//
// A destructive selector must skip such a row. Without that, it picks the same
// rows every pass and audits each erase again. A batch limit then fills with
// them before it reaches mail that still holds content.
func ActivityErasedSQL(activity string) string {
	return fmt.Sprintf(`(%[1]s.subject = '%[2]s' AND %[1]s.body IS NULL AND %[1]s.raw IS NULL)`,
		activity, ErasedActivitySubject)
}

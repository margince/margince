// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The reasons an undo leaves an import-created row in place. They sit apart
// from the catalog to keep each file under the length cap, and withUndoReasons
// joins the two.

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Reason codes for rows an undo keeps.
const (
	ReasonUndoEdited          ReasonCode = "undo_edited"
	ReasonUndoStillReferenced ReasonCode = "undo_still_referenced"
)

// withUndoReasons adds the undo reasons to the catalog the other reasons
// declare, and answers it.
func withUndoReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	maps.Copy(catalog, undoReasons)
	return catalog
}

var undoReasons = map[ReasonCode]reasonEntry{
	ReasonUndoEdited: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Someone changed this record after the import, so it was left in place.",
		textlang.German:     "Jemand hat diesen Datensatz nach dem Import geändert, daher blieb er bestehen.",
		textlang.Vietnamese: "Có người đã sửa hồ sơ này sau khi nhập nên hồ sơ được giữ nguyên.",
	}},
	ReasonUndoStillReferenced: {params: []string{"deals", "contacts"}, text: map[textlang.Lang]string{
		textlang.English:    "Live deals or contacts still name this company (deals: {deals}, contacts: {contacts}), so it was left in place.",
		textlang.German:     "Aktive Deals oder Kontakte nennen dieses Unternehmen noch (Deals: {deals}, Kontakte: {contacts}), daher blieb es bestehen.",
		textlang.Vietnamese: "Vẫn còn deal hoặc liên hệ đang dùng công ty này (deal: {deals}, liên hệ: {contacts}) nên công ty được giữ nguyên.",
	}},
}

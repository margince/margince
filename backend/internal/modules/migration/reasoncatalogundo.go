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
	// ReasonRunInFlight is not a row's reason: it answers a press on a run
	// another attempt already holds.
	ReasonRunInFlight ReasonCode = "run_in_flight"
)

// Parameter names of the still-referenced reason.
const (
	paramDeals    = "deals"
	paramContacts = "contacts"
)

// withUndoReasons adds the undo reasons to the catalog the other reasons
// declare, and answers it.
func withUndoReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	maps.Copy(catalog, undoReasons)
	return catalog
}

var undoReasons = map[ReasonCode]reasonEntry{
	ReasonRunInFlight: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This import is already running. It finishes on its own and this page updates.",
		textlang.German:     "Dieser Import läuft bereits. Er endet von selbst und diese Seite aktualisiert sich.",
		textlang.Vietnamese: "Lần nhập này đang chạy. Nó tự hoàn tất và trang này tự cập nhật.",
	}},
	ReasonUndoEdited: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Someone changed this record after the import, so it was left in place.",
		textlang.German:     "Jemand hat diesen Datensatz nach dem Import geändert, daher blieb er bestehen.",
		textlang.Vietnamese: "Bản ghi này đã được sửa sau khi nhập nên được giữ nguyên.",
	}},
	ReasonUndoStillReferenced: {params: []string{paramDeals, paramContacts}, text: map[textlang.Lang]string{
		textlang.English:    "Live deals or contacts still name this company (deals: {deals}, contacts: {contacts}), so it was left in place.",
		textlang.German:     "Aktive Deals oder Kontakte nennen dieses Unternehmen noch (Deals: {deals}, Kontakte: {contacts}), daher blieb es bestehen.",
		textlang.Vietnamese: "Vẫn còn deal hoặc liên hệ đang dùng công ty này (deal: {deals}, liên hệ: {contacts}) nên công ty được giữ nguyên.",
	}},
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var blockedWords = struct{ RoleReadsOnly, Complete, Settle, Deal, ReadOnlyContact langcopy.Phrase }{
	RoleReadsOnly:   langcopy.Phrase{En: "Your role can only read. Ask an administrator for another role.", De: "Deine Rolle erlaubt nur das Lesen. Frage einen Admin nach einer anderen Rolle.", Vi: "Vai trò của bạn chỉ có quyền xem. Liên hệ quản trị viên để được cấp vai trò khác."},
	ReadOnlyContact: langcopy.Phrase{En: "You can only read this contact. Ask its owner for write access.", De: "Du kannst diesen Kontakt nur lesen. Frage das zuständige Teammitglied nach Schreibzugriff.", Vi: "Bạn chỉ có quyền xem liên hệ này. Hãy nhờ chủ sở hữu cấp quyền ghi."},
	Complete:        langcopy.Phrase{En: "You do not have permission to complete tasks", De: "Sie haben keine Berechtigung, Aufgaben abzuschließen", Vi: "Bạn không có quyền hoàn thành nhiệm vụ"},
	Deal:            langcopy.Phrase{En: "You do not have permission to change deals", De: "Sie haben keine Berechtigung, Deals zu ändern", Vi: "Bạn không có quyền thay đổi giao dịch"},
	Settle:          langcopy.Phrase{En: "You do not have permission to settle commitments", De: "Sie haben keine Berechtigung, Zusagen abzuschließen", Vi: "Bạn không có quyền hoàn tất cam kết"},
}

// RoleReadsOnly is the refusal for a reader whose seat or role may not write an
// activity. gates/frontendrolereadsonly_test.go holds it equal to the frontend's
// record.roleReadsOnly, which the record pages show for the same refusal.
func RoleReadsOnly() langcopy.Phrase { return blockedWords.RoleReadsOnly }

// grantRefusal words a write the reader's grants refuse. A reader who may not
// write even an activity has a role that only reads, whatever the verb asked.
func grantRefusal(ctx context.Context, said langcopy.Spoken, specific langcopy.Phrase) string {
	if !auth.AllowsWrite(ctx, "activity", principal.ActionCreate) {
		return said.Say(blockedWords.RoleReadsOnly)
	}
	return said.Say(specific)
}

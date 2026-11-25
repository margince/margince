// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var blockedWords = struct{ SeatReadsOnly, RoleReadsOnly, ActionRefused, Complete, Settle, ReadOnlyContact langcopy.Phrase }{
	SeatReadsOnly:   langcopy.Phrase{En: "Your seat is read-only. Ask an admin for a full seat.", De: "Du hast einen Leseplatz. Frage einen Admin nach einem vollen Platz.", Vi: "Tài khoản của bạn chỉ có quyền xem. Liên hệ quản trị viên để được cấp tài khoản đầy đủ quyền."},
	RoleReadsOnly:   langcopy.Phrase{En: "Your role can only read. Ask an administrator for another role.", De: "Deine Rolle erlaubt nur das Lesen. Frage einen Admin nach einer anderen Rolle.", Vi: "Vai trò của bạn chỉ có quyền xem. Liên hệ quản trị viên để được cấp vai trò khác."},
	ActionRefused:   langcopy.Phrase{En: "Your role does not include this action. Ask an administrator for the right.", De: "Deine Rolle enthält diese Aktion nicht. Frage einen Admin nach dem Recht.", Vi: "Vai trò của bạn không bao gồm thao tác này. Liên hệ quản trị viên để được cấp thêm quyền."},
	ReadOnlyContact: langcopy.Phrase{En: "You can only read this contact. Ask its owner for write access.", De: "Du kannst diesen Kontakt nur lesen. Frage das zuständige Teammitglied nach Schreibzugriff.", Vi: "Bạn chỉ có quyền xem liên hệ này. Hãy nhờ chủ sở hữu cấp quyền ghi."},
	Complete:        langcopy.Phrase{En: "You do not have permission to complete tasks", De: "Sie haben keine Berechtigung, Aufgaben abzuschließen", Vi: "Bạn không có quyền hoàn thành nhiệm vụ"},
	Settle:          langcopy.Phrase{En: "You do not have permission to settle commitments", De: "Sie haben keine Berechtigung, Zusagen abzuschließen", Vi: "Bạn không có quyền hoàn tất cam kết"},
}

// SeatReadsOnly is the refusal for a read seat. gates/frontendrolereadsonly_test.go
// holds it, RoleReadsOnly and ActionRefused equal to settings.seatRefused,
// record.roleReadsOnly and record.actionRefused.
func SeatReadsOnly() langcopy.Phrase { return blockedWords.SeatReadsOnly }

// RoleReadsOnly is the refusal for a role that can only read.
func RoleReadsOnly() langcopy.Phrase { return blockedWords.RoleReadsOnly }

// ActionRefused is the refusal for an action the role does not include.
func ActionRefused() langcopy.Phrase { return blockedWords.ActionRefused }

// writeRefusal words a refused write in the record pages' order. The seat comes
// first, then a role that may not change the record the verb sits on. Last is
// the grant this verb alone needs.
func writeRefusal(ctx context.Context, said langcopy.Spoken, record string, specific langcopy.Phrase) string {
	switch {
	case !auth.SeatAdmitsWrites(ctx):
		return said.Say(blockedWords.SeatReadsOnly)
	case record != "" && !auth.Allows(ctx, record, principal.ActionUpdate):
		return said.Say(blockedWords.RoleReadsOnly)
	}
	return said.Say(specific)
}

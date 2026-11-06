// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var blockedWords = struct{ Log, Book, Complete, Settle, ReadOnlyContact langcopy.Phrase }{
	ReadOnlyContact: langcopy.Phrase{En: "You can only read this contact. Ask its owner for write access.", De: "Du kannst diesen Kontakt nur lesen. Frage das zuständige Teammitglied nach Schreibzugriff.", Vi: "Bạn chỉ có quyền xem liên hệ này. Hãy nhờ chủ sở hữu cấp quyền ghi."},
	Log:             langcopy.Phrase{En: "You do not have permission to log activities", De: "Sie haben keine Berechtigung, Aktivitäten zu erfassen", Vi: "Bạn không có quyền ghi lại hoạt động"},
	Book:            langcopy.Phrase{En: "You do not have permission to book meetings", De: "Sie haben keine Berechtigung, Termine zu buchen", Vi: "Bạn không có quyền đặt lịch họp"},
	Complete:        langcopy.Phrase{En: "You do not have permission to complete tasks", De: "Sie haben keine Berechtigung, Aufgaben abzuschließen", Vi: "Bạn không có quyền hoàn thành nhiệm vụ"},
	Settle:          langcopy.Phrase{En: "You do not have permission to settle commitments", De: "Sie haben keine Berechtigung, Zusagen abzuschließen", Vi: "Bạn không có quyền hoàn tất cam kết"},
}

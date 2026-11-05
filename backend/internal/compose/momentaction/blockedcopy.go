// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var blockedWords = struct{ Log, Complete, Settle langcopy.Phrase }{
	Log:      langcopy.Phrase{En: "You do not have permission to log activities", De: "Sie haben keine Berechtigung, Aktivitäten zu erfassen", Vi: "Bạn không có quyền ghi lại hoạt động"},
	Complete: langcopy.Phrase{En: "You do not have permission to complete tasks", De: "Sie haben keine Berechtigung, Aufgaben abzuschließen", Vi: "Bạn không có quyền hoàn thành nhiệm vụ"},
	Settle:   langcopy.Phrase{En: "You do not have permission to settle commitments", De: "Sie haben keine Berechtigung, Zusagen abzuschließen", Vi: "Bạn không có quyền hoàn tất cam kết"},
}

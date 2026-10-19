// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var lateWords = struct{ Hours, Day, Days langcopy.Phrase }{
	Day:   langcopy.Phrase{En: "Due one day ago and still open.", De: "Seit einem Tag fällig und noch offen.", Vi: "Quá hạn một ngày và vẫn đang mở."},
	Hours: langcopy.Phrase{En: "Due less than a day ago and still open.", De: "Seit weniger als einem Tag fällig und noch offen.", Vi: "Quá hạn chưa đến một ngày và vẫn đang mở."},
	Days:  langcopy.Phrase{En: "Due %d days ago and still open.", De: "Seit %d Tagen fällig und noch offen.", Vi: "Quá hạn %d ngày và vẫn đang mở."},
}

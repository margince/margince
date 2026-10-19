// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var captureWords = struct{ Failed langcopy.Phrase }{
	Failed: langcopy.Phrase{
		En: "Capture failed. Check the configured team and pipeline, then retry the forecast capture job.",
		De: "Die Erfassung ist fehlgeschlagen. Prüfe das konfigurierte Team und die Pipeline und starte die Prognoseerfassung erneut.",
		Vi: "Không thể ghi nhận dữ liệu. Kiểm tra nhóm và pipeline đã cấu hình, rồi chạy lại tác vụ ghi nhận dự báo.",
	},
}

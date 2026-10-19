// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var reportingWords = struct{ Range, Unpriced, Collecting, RestrictedHistory langcopy.Phrase }{
	Range:             langcopy.Phrase{En: "The total exceeds the exact display range", De: "Die Summe überschreitet den exakt darstellbaren Zahlenbereich.", Vi: "Tổng vượt quá phạm vi số có thể hiển thị chính xác."},
	Unpriced:          langcopy.Phrase{En: "Some contributions have no available value", De: "Für einige Beiträge ist kein Wert verfügbar.", Vi: "Một số khoản đóng góp chưa có giá trị khả dụng."},
	Collecting:        langcopy.Phrase{En: "Collecting movement history", De: "Die Änderungshistorie wird gesammelt.", Vi: "Đang thu thập lịch sử biến động."},
	RestrictedHistory: langcopy.Phrase{En: "Some historical evidence is restricted; this reading cannot be compared", De: "Einige historische Belege sind zugriffsbeschränkt. Diese Auswertung kann nicht verglichen werden.", Vi: "Một số bằng chứng lịch sử bị hạn chế truy cập; không thể so sánh kết quả này."},
}

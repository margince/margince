// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var scanWords = struct{ NoWorker, Denied, Failed, Invalid, NoModel langcopy.Phrase }{
	NoWorker: langcopy.Phrase{En: "No worker runs account scans here, so only the rule-based advice is shown.", De: "Es ist kein Worker für die Unternehmensanalyse verfügbar. Daher werden nur regelbasierte Empfehlungen angezeigt.", Vi: "Không có tiến trình xử lý phân tích khách hàng, nên chỉ hiển thị các đề xuất dựa trên quy tắc."},
	Denied:   langcopy.Phrase{En: "The account could not be opened for this reader, so it was not read.", De: "Dieses Unternehmen konnte für dich nicht geöffnet werden und wurde daher nicht analysiert.", Vi: "Không thể mở hồ sơ khách hàng này với quyền truy cập của bạn, nên hồ sơ chưa được phân tích."},
	Failed:   langcopy.Phrase{En: "The account could not be read. Try again later.", De: "Das Unternehmen konnte nicht analysiert werden. Versuche es später erneut.", Vi: "Không thể phân tích hồ sơ khách hàng. Hãy thử lại sau."},
	Invalid:  langcopy.Phrase{En: "The model's answer didn't match the records, so only the rule-based advice is shown.", De: "Die Antwort des Modells stimmte nicht mit den erfassten Informationen überein. Daher werden nur regelbasierte Empfehlungen angezeigt.", Vi: "Câu trả lời của mô hình không khớp với hồ sơ, nên chỉ hiển thị các đề xuất dựa trên quy tắc."},
	NoModel:  langcopy.Phrase{En: "No model is configured here, so only the rule-based advice is shown.", De: "Es ist kein Modell eingerichtet. Daher werden nur regelbasierte Empfehlungen angezeigt.", Vi: "Chưa có mô hình nào được cấu hình, nên chỉ hiển thị các đề xuất dựa trên quy tắc."},
}

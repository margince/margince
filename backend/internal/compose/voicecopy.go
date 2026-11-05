// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

type voiceCopy struct{ langcopy.Spoken }

func voiceCopyFor(language string) voiceCopy { return voiceCopy{langcopy.For(language)} }

var voiceReviewWords = struct{ Uncomparable, Identity, Signature, Removed, HardFailures, Malformed, BelowFloor, Regression, TooSmall langcopy.Phrase }{
	Uncomparable: langcopy.Phrase{En: "the previous version's stored profile is not comparable; review before replacing it", De: "Das gespeicherte Profil der vorherigen Version ist nicht vergleichbar. Prüfe es vor dem Ersetzen.", Vi: "Không thể so sánh hồ sơ đã lưu của phiên bản trước; hãy kiểm tra trước khi thay thế."},
	Identity:     langcopy.Phrase{En: "identity vocabulary shifted (jaccard %.2f)", De: "Das charakteristische Vokabular hat sich verändert (Jaccard-Wert %.2f).", Vi: "Từ vựng đặc trưng đã thay đổi (hệ số Jaccard %.2f)."},
	Signature:    langcopy.Phrase{En: "signature moves shifted (jaccard %.2f)", De: "Die typischen Stilmittel haben sich verändert (Jaccard-Wert %.2f).", Vi: "Các cách diễn đạt đặc trưng đã thay đổi (hệ số Jaccard %.2f)."},
	Removed:      langcopy.Phrase{En: "Removed rules: %d avoid, %d register", De: "Entfernte Regeln: %d zur Wortwahl, %d zum Sprachregister.", Vi: "Đã xóa %d quy tắc tránh diễn đạt và %d quy tắc về văn phong."},
	HardFailures: langcopy.Phrase{En: "Anti-AI hard failures remaining after sanitizing: %d", De: "Nach der Bereinigung verbleibende schwerwiegende Verstöße gegen die Regeln für natürliche Sprache: %d.", Vi: "Sau khi làm sạch, vẫn còn %d lỗi nghiêm trọng về cách diễn đạt mang dấu hiệu AI."},
	Malformed:    langcopy.Phrase{En: "the model returned malformed drafts during evaluation", De: "Das Modell lieferte bei der Bewertung Entwürfe mit ungültiger Struktur.", Vi: "Mô hình trả về bản nháp sai cấu trúc trong quá trình đánh giá."},
	BelowFloor:   langcopy.Phrase{En: "median voice score %.2f is below the %.2f floor", De: "Der Median der Stilbewertung %.2f liegt unter dem Mindestwert %.2f.", Vi: "Điểm phong cách trung vị %.2f thấp hơn ngưỡng %.2f."},
	Regression:   langcopy.Phrase{En: "candidate scores %.2f against the active version's %.2f", De: "Der Kandidat erreicht %.2f gegenüber %.2f bei der aktiven Version.", Vi: "Phiên bản đề xuất đạt %.2f so với %.2f của phiên bản đang dùng."},
	TooSmall:     langcopy.Phrase{En: "the corpus is too small to reserve held-out evaluation samples; add more sources to enable scoring", De: "Die Textsammlung ist zu klein, um getrennte Bewertungsbeispiele zurückzuhalten. Füge weitere Quellen hinzu, um die Bewertung zu ermöglichen.", Vi: "Tập văn bản quá nhỏ để dành mẫu riêng cho đánh giá; hãy thêm nguồn để có thể chấm điểm."},
}

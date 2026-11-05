// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var voiceCompletionWords = struct{ CorpusChanged, ActiveChanged langcopy.Phrase }{
	CorpusChanged: langcopy.Phrase{En: "the corpus changed while this build was running; review before activating", De: "Die Textsammlung hat sich während der Erstellung geändert. Prüfe die Version vor dem Aktivieren.", Vi: "Tập văn bản đã thay đổi trong khi tạo; hãy kiểm tra trước khi kích hoạt."},
	ActiveChanged: langcopy.Phrase{En: "the active version changed while this build was evaluating; review before activating", De: "Die aktive Version hat sich während der Bewertung geändert. Prüfe die neue Version vor dem Aktivieren.", Vi: "Phiên bản đang dùng đã thay đổi trong khi đánh giá; hãy kiểm tra trước khi kích hoạt."},
}

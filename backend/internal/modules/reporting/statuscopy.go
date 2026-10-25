// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// WithBaseLanguage uses the writer's transaction rather than acquiring another connection.
func (s *Service) WithBaseLanguage(resolve func(context.Context, pgx.Tx) textlang.Lang) *Service {
	configured := *s
	configured.language = resolve
	return &configured
}

// WithReaderLanguage names the language of text only the caller reads and
// nothing stores. Stored run statuses keep the base language.
func (s *Service) WithReaderLanguage(resolve func(context.Context, pgx.Tx) textlang.Lang) *Service {
	configured := *s
	configured.reader = resolve
	return &configured
}

// readerCopy is the caller's language for a live read, else the status language.
func (s *Service) readerCopy(ctx context.Context, tx pgx.Tx) langcopy.Spoken {
	if s.reader == nil {
		return s.statusCopy(ctx, tx)
	}
	return langcopy.For(string(s.reader(ctx, tx)))
}

func (s *Service) statusCopy(ctx context.Context, tx pgx.Tx) langcopy.Spoken {
	language := textlang.English
	if s.language != nil {
		language = s.language(ctx, tx)
	}
	return langcopy.For(string(language))
}

var statusWords = struct{ Failed, Exhausted, Access, Changed, Timezone, Paused, Unconfigured, SkippedOne, SkippedMany langcopy.Phrase }{
	SkippedOne:   langcopy.Phrase{En: "Skipped %d overdue period from %s through %s; newest %d retained", De: "%d überfälliger Zeitraum von %s bis %s übersprungen; die letzten %d bleiben erhalten", Vi: "Đã bỏ qua %d kỳ quá hạn từ %s đến %s; giữ lại %d kỳ gần nhất"},
	SkippedMany:  langcopy.Phrase{En: "Skipped %d overdue periods from %s through %s; newest %d retained", De: "%d überfällige Zeiträume von %s bis %s übersprungen; die letzten %d bleiben erhalten", Vi: "Đã bỏ qua %d kỳ quá hạn từ %s đến %s; giữ lại %d kỳ gần nhất"},
	Unconfigured: langcopy.Phrase{En: "Not configured", De: "Nicht eingerichtet", Vi: "Chưa thiết lập"},
	Failed:       langcopy.Phrase{En: "Report generation failed. Retry the run.", De: "Die Berichtserstellung ist fehlgeschlagen. Starte den Lauf erneut.", Vi: "Tạo báo cáo thất bại. Hãy chạy lại."},
	Exhausted:    langcopy.Phrase{En: "Report generation stopped repeatedly before completing. Retry the run.", De: "Die Berichtserstellung wurde mehrfach vor dem Abschluss unterbrochen. Starte den Lauf erneut.", Vi: "Quá trình tạo báo cáo liên tục dừng trước khi hoàn tất. Hãy chạy lại."},
	Access:       langcopy.Phrase{En: "The accountable owner can no longer generate this report. Review their access and schedule.", De: "Die verantwortliche Person kann diesen Bericht nicht mehr erstellen. Prüfe ihre Zugriffsrechte und den Zeitplan.", Vi: "Người phụ trách không còn có thể tạo báo cáo này. Kiểm tra quyền truy cập và lịch chạy của họ."},
	Changed:      langcopy.Phrase{En: "The report or schedule changed before publication.", De: "Der Bericht oder Zeitplan wurde vor der Veröffentlichung geändert.", Vi: "Báo cáo hoặc lịch chạy đã thay đổi trước khi xuất bản."},
	Timezone:     langcopy.Phrase{En: "Reporting timezone changed; review and save the schedule", De: "Die Berichtszeitzone hat sich geändert. Prüfe und speichere den Zeitplan.", Vi: "Múi giờ báo cáo đã thay đổi; kiểm tra và lưu lịch chạy"},
	Paused:       langcopy.Phrase{En: "Paused by the reporting administrator; explicit resume required", De: "Von der Berichtsverwaltung pausiert. Der Zeitplan muss ausdrücklich fortgesetzt werden.", Vi: "Đã được quản trị viên báo cáo tạm dừng; cần chủ động tiếp tục"},
}
